// Copyright 2019-2020 Kosc Telecom.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dispatcher

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"horus/log"

	"github.com/jmoiron/sqlx"
)

var (
	db, lockDB               *sqlx.DB
	appLockConn              *sql.Conn
	lockDevStmt              *sql.Stmt
	unlockDevStmt            *sql.Stmt
	unlockAllDevStmt         *sql.Stmt
	unlockDevFromReportStmt  *sql.Stmt
	unlockFromOngoingStmt    *sql.Stmt
	setDevLastPolledAt       *sql.Stmt
	setDevLastPingedAt       *sql.Stmt
	insertMetricLastPolledAt *sql.Stmt
	checkAgentStmt           *sql.Stmt
)

// ConnectDB connects to postgres db
func ConnectDB(dsn, lockDSN string) error {
	var err error

	db, err = dbOpen(dsn)
	if err != nil || lockDSN == "" {
		return err
	}
	lockDB, err = dbOpen(lockDSN)
	return err
}

func AcquireLock(ctx context.Context, lockID int) error {
	var err error

	appLockConn, err = lockDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("lock db conn: %v", err)
	}
	log.Infof("querying advisory lock from pg...")
	_, err = appLockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockID)
	if err != nil {
		return fmt.Errorf("select pg_advisory_lock: %v", err)
	}
	log.Infof("lock granted, running as master!")
	go func(ctx context.Context) {
		log.Debug2f("starting db lock conn pinger")
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			if _, err := appLockConn.ExecContext(ctx, `SELECT 1`); err != nil {
				log.Exitf("db lock conn ping: %v", err)
			}
		}
	}(ctx)
	return nil
}

// PrepareQueries prepares the db queries
func PrepareQueries() error {
	var err error

	lockDevStmt, err = db.Prepare(`UPDATE devices
                                      SET is_polling = true
                                    WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("prepare lockDevStmt: %v", err)
	}
	unlockDevStmt, err = db.Prepare(`UPDATE devices
                                        SET is_polling = false
                                      WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("prepare unlockDevStmt: %v", err)
	}
	unlockAllDevStmt, err = db.Prepare(`UPDATE devices
                                           SET is_polling = false
                                         WHERE last_polled_at < NOW() - ($1::TEXT || ' seconds')::INTERVAL
                                           AND is_polling = true`)
	if err != nil {
		return fmt.Errorf("prepare unlockAllDevStmt: %v", err)
	}
	unlockDevFromReportStmt, err = db.Prepare(`UPDATE devices
                                                  SET is_polling = false
                                                WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("prepare unlockDevFromReportStmt: %v", err)
	}
	unlockFromOngoingStmt, err = db.Prepare(`UPDATE devices
                                                SET is_polling = false
                                              WHERE last_polled_at < NOW() - (polling_frequency::TEXT || ' seconds')::INTERVAL
                                                AND NOT (id = ANY($1::int[]))`)
	if err != nil {
		return fmt.Errorf("prepare unlockFromOngoingStmt: %v", err)
	}
	setDevLastPolledAt, err = db.Prepare(`UPDATE devices
                                             SET last_polled_at = NOW()
                                           WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("prepare setLastPollDate: %v", err)
	}
	setDevLastPingedAt, err = db.Prepare(`UPDATE devices
                                             SET last_pinged_at = NOW()
                                           WHERE id = ANY($1)`)
	if err != nil {
		return fmt.Errorf("prepare setLastPingDate: %v", err)
	}
	insertMetricLastPolledAt, err = db.Prepare(`INSERT INTO metric_poll_times
                                                            (device_id, metric_id, last_polled_at)
                                                     VALUES ($1, $2, NOW())
                                                ON CONFLICT (device_id, metric_id)
                                                  DO UPDATE
                                                        SET last_polled_at = NOW()`)
	if err != nil {
		return fmt.Errorf("prepare insertMetricLastPolledAt: %v", err)
	}
	checkAgentStmt, err = db.Prepare(`UPDATE agents
                                         SET last_checked_at = NOW(),
                                             is_alive = $2,
                                             load = $3
                                       WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("prepare checkAgentStmt: %v", err)
	}
	return nil
}

// ReleaseDB closes the db connection.
func ReleaseDB() {
	db.Close()
	if lockDB != nil {
		lockDB.Close()
	}
}

// sqlExec executes the prepared statement stmt with its args,
// and logs and returns the db error if any
func sqlExec(id interface{}, reqName string, stmt *sql.Stmt, args ...interface{}) error {
	log.Debug3f("%v - sql exec %s", id, reqName)
	if stmt == nil {
		log.Errorf("sql exec %s (%v): nil stmt", reqName, id)
		return fmt.Errorf("sql exec %s: nil stmt", reqName)
	}
	_, err := stmt.Exec(args...)
	if err != nil {
		log.Errorf("sql exec %s (%v): %v", reqName, id, err)
	}
	return err
}

func dbOpen(dsn string) (*sqlx.DB, error) {
	var err error
	var dbh *sqlx.DB

	log.Debug2f("opening db connection to %q", dsn)
	dbh, err = sqlx.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect db %q: %v", dsn, err)
	}
	return dbh, err
}
