// HomeLink - Event Persistence and Database Storage
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// StorageConfig configures the event storage system
type StorageConfig struct {
	Enabled        bool   `json:"enabled"`
	DatabasePath   string `json:"database_path"`   // Path to SQLite database file
	RetentionDays  int    `json:"retention_days"`  // Days to keep events (0 = forever)
	MaxEvents      int    `json:"max_events"`      // Maximum events to store (0 = unlimited)
	BatchSize      int    `json:"batch_size"`      // Events to batch insert
	FlushInterval  time.Duration `json:"flush_interval"`  // How often to flush batched events
	BackupEnabled  bool   `json:"backup_enabled"`
	BackupPath     string `json:"backup_path"`
	BackupInterval time.Duration `json:"backup_interval"`
}

// EventStorage manages persistent event storage
type EventStorage struct {
	config    StorageConfig
	db        *sql.DB
	mutex     sync.RWMutex
	eventBatch []StoredEvent
	stats     StorageStats
	stopChan  chan bool
}

// StoredEvent represents an event in the database
type StoredEvent struct {
	ID           int64                  `json:"id"`
	DeviceID     string                 `json:"device_id"`
	EventType    string                 `json:"event_type"`
	Timestamp    int64                  `json:"timestamp"`
	Data         map[string]interface{} `json:"data"`
	Priority     string                 `json:"priority"`
	ProcessedBy  string                 `json:"processed_by"`  // Which filters/rules processed it
	Tags         []string               `json:"tags"`
	CreatedAt    time.Time              `json:"created_at"`
	ExpiresAt    *time.Time             `json:"expires_at"`
}

// StorageStats tracks storage system performance
type StorageStats struct {
	EventsStored        uint64    `json:"events_stored"`
	EventsDeleted       uint64    `json:"events_deleted"`
	DatabaseSize        int64     `json:"database_size_bytes"`
	LastCleanup         time.Time `json:"last_cleanup"`
	LastBackup          time.Time `json:"last_backup"`
	BatchesProcessed    uint64    `json:"batches_processed"`
	StorageErrors       uint64    `json:"storage_errors"`
	AverageInsertTime   int64     `json:"average_insert_time_ms"`
	OldestEvent         time.Time `json:"oldest_event"`
	NewestEvent         time.Time `json:"newest_event"`
}

// EventQuery represents a query for stored events
type EventQuery struct {
	DeviceIDs    []string  `json:"device_ids"`
	EventTypes   []string  `json:"event_types"`
	StartTime    *time.Time `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	Tags         []string  `json:"tags"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
	OrderBy      string    `json:"order_by"`    // "timestamp", "device_id", "event_type"
	OrderDesc    bool      `json:"order_desc"`
}

// EventAggregate represents aggregated event statistics
type EventAggregate struct {
	DeviceID      string    `json:"device_id"`
	EventType     string    `json:"event_type"`
	Count         int64     `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	AverageValue  *float64  `json:"average_value"`  // For numeric data fields
}

// NewEventStorage creates a new event storage system
func NewEventStorage(config StorageConfig) (*EventStorage, error) {
	storage := &EventStorage{
		config:     config,
		eventBatch: make([]StoredEvent, 0, config.BatchSize),
		stopChan:   make(chan bool),
		stats:      StorageStats{},
	}

	if config.Enabled {
		if err := storage.initDatabase(); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %v", err)
		}

		// Start background processes
		go storage.batchProcessor()
		go storage.cleanupRoutine()
		
		if config.BackupEnabled {
			go storage.backupRoutine()
		}

		log.Printf("Event storage initialized with database: %s", config.DatabasePath)
		log.Printf("  Retention: %d days, Max events: %d", config.RetentionDays, config.MaxEvents)
		log.Printf("  Batch size: %d, Flush interval: %v", config.BatchSize, config.FlushInterval)
	}

	return storage, nil
}

// initDatabase initializes the SQLite database and creates tables
func (es *EventStorage) initDatabase() error {
	var err error
	es.db, err = sql.Open("sqlite3", es.config.DatabasePath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Set connection pool settings
	es.db.SetMaxOpenConns(25)
	es.db.SetMaxIdleConns(5)
	es.db.SetConnMaxLifetime(5 * time.Minute)

	// Create tables
	if err := es.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %v", err)
	}

	// Create indexes
	if err := es.createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	return nil
}

// createTables creates the required database tables
func (es *EventStorage) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			data TEXT NOT NULL,
			priority TEXT DEFAULT 'normal',
			processed_by TEXT,
			tags TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS device_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT UNIQUE NOT NULL,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			event_count INTEGER DEFAULT 0,
			last_event_type TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS storage_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := es.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %v", err)
		}
	}

	return nil
}

// createIndexes creates database indexes for better query performance
func (es *EventStorage) createIndexes() error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_events_device_timestamp ON events(device_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type_timestamp ON events(event_type, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_expires_at ON events(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_device_stats_device_id ON device_stats(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_device_stats_last_seen ON device_stats(last_seen DESC)`,
	}

	for _, index := range indexes {
		if _, err := es.db.Exec(index); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	return nil
}

// StoreEvent stores an event in the database
func (es *EventStorage) StoreEvent(msg *Message) error {
	if !es.config.Enabled {
		return nil
	}

	// Convert message to stored event
	dataJSON, err := json.Marshal(msg.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %v", err)
	}

	storedEvent := StoredEvent{
		DeviceID:    msg.DeviceID,
		EventType:   string(msg.Type),
		Timestamp:   msg.Timestamp,
		Data:        msg.Data.(map[string]interface{}),
		Priority:    "normal", // Default priority
		CreatedAt:   time.Now(),
	}

	// Set expiry time if retention is configured
	if es.config.RetentionDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, es.config.RetentionDays)
		storedEvent.ExpiresAt = &expiresAt
	}

	// Add to batch or store immediately
	if es.config.BatchSize > 1 {
		es.addToBatch(storedEvent)
	} else {
		return es.storeEventImmediate(storedEvent, string(dataJSON))
	}

	return nil
}

// addToBatch adds an event to the batch for later processing
func (es *EventStorage) addToBatch(event StoredEvent) {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	es.eventBatch = append(es.eventBatch, event)
	
	// Flush batch if it's full
	if len(es.eventBatch) >= es.config.BatchSize {
		go es.flushBatch()
	}
}

// storeEventImmediate stores a single event immediately
func (es *EventStorage) storeEventImmediate(event StoredEvent, dataJSON string) error {
	start := time.Now()
	
	tx, err := es.db.Begin()
	if err != nil {
		es.incrementStorageErrors()
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Insert event
	tagsJSON := ""
	if len(event.Tags) > 0 {
		tagsBytes, _ := json.Marshal(event.Tags)
		tagsJSON = string(tagsBytes)
	}

	_, err = tx.Exec(`
		INSERT INTO events (device_id, event_type, timestamp, data, priority, processed_by, tags, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.DeviceID, event.EventType, event.Timestamp, dataJSON, event.Priority,
		event.ProcessedBy, tagsJSON, event.CreatedAt, event.ExpiresAt)
	
	if err != nil {
		es.incrementStorageErrors()
		return fmt.Errorf("failed to insert event: %v", err)
	}

	// Update device stats
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO device_stats (device_id, first_seen, last_seen, event_count, last_event_type)
		VALUES (
			?,
			COALESCE((SELECT first_seen FROM device_stats WHERE device_id = ?), ?),
			?,
			COALESCE((SELECT event_count FROM device_stats WHERE device_id = ?), 0) + 1,
			?
		)`,
		event.DeviceID, event.DeviceID, event.CreatedAt, event.CreatedAt, event.DeviceID, event.EventType)
	
	if err != nil {
		es.incrementStorageErrors()
		return fmt.Errorf("failed to update device stats: %v", err)
	}

	if err = tx.Commit(); err != nil {
		es.incrementStorageErrors()
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	// Update statistics
	insertTime := time.Since(start).Milliseconds()
	es.mutex.Lock()
	es.stats.EventsStored++
	es.stats.AverageInsertTime = (es.stats.AverageInsertTime + insertTime) / 2
	es.mutex.Unlock()

	return nil
}

// flushBatch flushes the current batch of events to database
func (es *EventStorage) flushBatch() {
	es.mutex.Lock()
	if len(es.eventBatch) == 0 {
		es.mutex.Unlock()
		return
	}
	
	batch := make([]StoredEvent, len(es.eventBatch))
	copy(batch, es.eventBatch)
	es.eventBatch = es.eventBatch[:0] // Clear batch
	es.mutex.Unlock()

	start := time.Now()
	
	tx, err := es.db.Begin()
	if err != nil {
		es.incrementStorageErrors()
		log.Printf("Failed to begin batch transaction: %v", err)
		return
	}
	defer tx.Rollback()

	// Prepare statements
	eventStmt, err := tx.Prepare(`
		INSERT INTO events (device_id, event_type, timestamp, data, priority, processed_by, tags, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		es.incrementStorageErrors()
		log.Printf("Failed to prepare event statement: %v", err)
		return
	}
	defer eventStmt.Close()

	deviceStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO device_stats (device_id, first_seen, last_seen, event_count, last_event_type)
		VALUES (
			?,
			COALESCE((SELECT first_seen FROM device_stats WHERE device_id = ?), ?),
			?,
			COALESCE((SELECT event_count FROM device_stats WHERE device_id = ?), 0) + 1,
			?
		)`)
	if err != nil {
		es.incrementStorageErrors()
		log.Printf("Failed to prepare device statement: %v", err)
		return
	}
	defer deviceStmt.Close()

	// Insert all events in batch
	for _, event := range batch {
		dataJSON, err := json.Marshal(event.Data)
		if err != nil {
			continue
		}

		tagsJSON := ""
		if len(event.Tags) > 0 {
			tagsBytes, _ := json.Marshal(event.Tags)
			tagsJSON = string(tagsBytes)
		}

		_, err = eventStmt.Exec(event.DeviceID, event.EventType, event.Timestamp,
			string(dataJSON), event.Priority, event.ProcessedBy, tagsJSON,
			event.CreatedAt, event.ExpiresAt)
		if err != nil {
			log.Printf("Failed to insert batched event: %v", err)
			continue
		}

		_, err = deviceStmt.Exec(event.DeviceID, event.DeviceID, event.CreatedAt,
			event.CreatedAt, event.DeviceID, event.EventType)
		if err != nil {
			log.Printf("Failed to update device stats: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		es.incrementStorageErrors()
		log.Printf("Failed to commit batch transaction: %v", err)
		return
	}

	// Update statistics
	batchTime := time.Since(start).Milliseconds()
	es.mutex.Lock()
	es.stats.EventsStored += uint64(len(batch))
	es.stats.BatchesProcessed++
	es.stats.AverageInsertTime = (es.stats.AverageInsertTime + batchTime) / 2
	es.mutex.Unlock()

	log.Printf("Flushed batch of %d events in %dms", len(batch), batchTime)
}

// batchProcessor handles periodic batch flushing
func (es *EventStorage) batchProcessor() {
	ticker := time.NewTicker(es.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			es.flushBatch()
		case <-es.stopChan:
			// Final flush before stopping
			es.flushBatch()
			return
		}
	}
}

// QueryEvents retrieves events based on query criteria
func (es *EventStorage) QueryEvents(query EventQuery) ([]StoredEvent, error) {
	if !es.config.Enabled {
		return []StoredEvent{}, fmt.Errorf("storage not enabled")
	}

	// Build SQL query
	sqlQuery := "SELECT id, device_id, event_type, timestamp, data, priority, processed_by, tags, created_at, expires_at FROM events WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if len(query.DeviceIDs) > 0 {
		placeholders := make([]string, len(query.DeviceIDs))
		for i, deviceID := range query.DeviceIDs {
			placeholders[i] = "?"
			args = append(args, deviceID)
		}
		sqlQuery += fmt.Sprintf(" AND device_id IN (%s)", strings.Join(placeholders, ","))
	}

	if len(query.EventTypes) > 0 {
		placeholders := make([]string, len(query.EventTypes))
		for i, eventType := range query.EventTypes {
			placeholders[i] = "?"
			args = append(args, eventType)
		}
		sqlQuery += fmt.Sprintf(" AND event_type IN (%s)", strings.Join(placeholders, ","))
	}

	if query.StartTime != nil {
		sqlQuery += " AND timestamp >= ?"
		args = append(args, query.StartTime.Unix())
	}

	if query.EndTime != nil {
		sqlQuery += " AND timestamp <= ?"
		args = append(args, query.EndTime.Unix())
	}

	// Order by
	orderBy := "timestamp"
	if query.OrderBy != "" {
		orderBy = query.OrderBy
	}
	
	direction := "ASC"
	if query.OrderDesc {
		direction = "DESC"
	}
	
	sqlQuery += fmt.Sprintf(" ORDER BY %s %s", orderBy, direction)

	// Limit and offset
	if query.Limit > 0 {
		sqlQuery += " LIMIT ?"
		args = append(args, query.Limit)
		
		if query.Offset > 0 {
			sqlQuery += " OFFSET ?"
			args = append(args, query.Offset)
		}
	}

	rows, err := es.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %v", err)
	}
	defer rows.Close()

	var events []StoredEvent
	for rows.Next() {
		var event StoredEvent
		var dataJSON, tagsJSON sql.NullString

		err := rows.Scan(&event.ID, &event.DeviceID, &event.EventType,
			&event.Timestamp, &dataJSON, &event.Priority,
			&event.ProcessedBy, &tagsJSON, &event.CreatedAt, &event.ExpiresAt)
		if err != nil {
			continue
		}

		// Parse data JSON
		if dataJSON.Valid {
			json.Unmarshal([]byte(dataJSON.String), &event.Data)
		}

		// Parse tags JSON
		if tagsJSON.Valid {
			json.Unmarshal([]byte(tagsJSON.String), &event.Tags)
		}

		events = append(events, event)
	}

	return events, nil
}

// GetEventAggregates returns aggregated event statistics
func (es *EventStorage) GetEventAggregates(startTime, endTime time.Time) ([]EventAggregate, error) {
	if !es.config.Enabled {
		return []EventAggregate{}, fmt.Errorf("storage not enabled")
	}

	query := `
		SELECT device_id, event_type, COUNT(*) as count,
			   MIN(timestamp) as first_seen, MAX(timestamp) as last_seen
		FROM events
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY device_id, event_type
		ORDER BY count DESC, device_id, event_type`

	rows, err := es.db.Query(query, startTime.Unix(), endTime.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to query aggregates: %v", err)
	}
	defer rows.Close()

	var aggregates []EventAggregate
	for rows.Next() {
		var agg EventAggregate
		var firstSeenTS, lastSeenTS int64

		err := rows.Scan(&agg.DeviceID, &agg.EventType, &agg.Count,
			&firstSeenTS, &lastSeenTS)
		if err != nil {
			continue
		}

		agg.FirstSeen = time.Unix(firstSeenTS, 0)
		agg.LastSeen = time.Unix(lastSeenTS, 0)

		aggregates = append(aggregates, agg)
	}

	return aggregates, nil
}

// cleanupRoutine performs periodic cleanup of old events
func (es *EventStorage) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			es.performCleanup()
		case <-es.stopChan:
			return
		}
	}
}

// performCleanup removes expired events and enforces limits
func (es *EventStorage) performCleanup() {
	if !es.config.Enabled {
		return
	}

	start := time.Now()
	deletedCount := 0

	// Remove expired events
	if es.config.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -es.config.RetentionDays)
		result, err := es.db.Exec("DELETE FROM events WHERE created_at < ?", cutoff)
		if err != nil {
			log.Printf("Failed to delete expired events: %v", err)
		} else {
			if rows, err := result.RowsAffected(); err == nil {
				deletedCount += int(rows)
			}
		}
	}

	// Remove events beyond max limit
	if es.config.MaxEvents > 0 {
		result, err := es.db.Exec(`
			DELETE FROM events WHERE id NOT IN (
				SELECT id FROM events ORDER BY timestamp DESC LIMIT ?
			)`, es.config.MaxEvents)
		if err != nil {
			log.Printf("Failed to enforce max events limit: %v", err)
		} else {
			if rows, err := result.RowsAffected(); err == nil {
				deletedCount += int(rows)
			}
		}
	}

	// Update statistics
	es.mutex.Lock()
	es.stats.EventsDeleted += uint64(deletedCount)
	es.stats.LastCleanup = time.Now()
	es.mutex.Unlock()

	if deletedCount > 0 {
		log.Printf("Cleanup removed %d events in %v", deletedCount, time.Since(start))
	}
}

// backupRoutine performs periodic database backups
func (es *EventStorage) backupRoutine() {
	ticker := time.NewTicker(es.config.BackupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			es.performBackup()
		case <-es.stopChan:
			return
		}
	}
}

// performBackup creates a backup of the database
func (es *EventStorage) performBackup() {
	if !es.config.BackupEnabled || es.config.BackupPath == "" {
		return
	}

	start := time.Now()
	backupPath := fmt.Sprintf("%s/homelink_events_%s.db",
		es.config.BackupPath, time.Now().Format("2006-01-02_15-04-05"))

	_, err := es.db.Exec("VACUUM INTO ?", backupPath)
	if err != nil {
		log.Printf("Failed to create database backup: %v", err)
		return
	}

	es.mutex.Lock()
	es.stats.LastBackup = time.Now()
	es.mutex.Unlock()

	log.Printf("Database backup created: %s (took %v)", backupPath, time.Since(start))
}

// GetStats returns storage system statistics
func (es *EventStorage) GetStats() StorageStats {
	if !es.config.Enabled {
		return StorageStats{}
	}

	es.mutex.RLock()
	stats := es.stats
	es.mutex.RUnlock()

	// Get database size
	if fileInfo, err := os.Stat(es.config.DatabasePath); err == nil {
		stats.DatabaseSize = fileInfo.Size()
	}

	// Get oldest and newest events
	var oldestTS, newestTS sql.NullInt64
	es.db.QueryRow("SELECT MIN(timestamp), MAX(timestamp) FROM events").Scan(&oldestTS, &newestTS)
	
	if oldestTS.Valid {
		stats.OldestEvent = time.Unix(oldestTS.Int64, 0)
	}
	if newestTS.Valid {
		stats.NewestEvent = time.Unix(newestTS.Int64, 0)
	}

	return stats
}

// incrementStorageErrors safely increments error counter
func (es *EventStorage) incrementStorageErrors() {
	es.mutex.Lock()
	es.stats.StorageErrors++
	es.mutex.Unlock()
}

// Stop gracefully stops the storage system
func (es *EventStorage) Stop() {
	if !es.config.Enabled {
		return
	}

	close(es.stopChan)
	
	// Final flush
	es.flushBatch()

	if es.db != nil {
		es.db.Close()
	}

	log.Printf("Event storage system stopped")
}

// IsEnabled returns whether storage is enabled
func (es *EventStorage) IsEnabled() bool {
	return es.config.Enabled
}