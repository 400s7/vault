package connutil

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault/helper/parseutil"
	"github.com/hashicorp/vault/plugins/helper/database/dbutil"
	"github.com/mitchellh/mapstructure"
)

var _ ConnectionProducer = &SQLConnectionProducer{}

// SQLConfig contains the config options for SQL database engines
type SQLConfig struct {
	ConnectionURL            string      `json:"connection_url" mapstructure:"connection_url"`
	MaxOpenConnections       int         `json:"max_open_connections" mapstructure:"max_open_connections"`
	MaxIdleConnections       int         `json:"max_idle_connections" mapstructure:"max_idle_connections"`
	MaxConnectionLifetimeRaw interface{} `json:"max_connection_lifetime" mapstructure:"max_connection_lifetime"`
	Username                 string      `json:"username" mapstructure:"username"`
	Password                 string      `json:"password" mapstructure:"password"`
}

// SQLConnectionProducer implements ConnectionProducer and provides a generic producer for most sql databases
type SQLConnectionProducer struct {
	ConnectionURL            string      `json:"connection_url" mapstructure:"connection_url"`
	MaxOpenConnections       int         `json:"max_open_connections" mapstructure:"max_open_connections"`
	MaxIdleConnections       int         `json:"max_idle_connections" mapstructure:"max_idle_connections"`
	MaxConnectionLifetimeRaw interface{} `json:"max_connection_lifetime" mapstructure:"max_connection_lifetime"`
	Username                 string      `json:"username" mapstructure:"username"`
	Password                 string      `json:"password" mapstructure:"password"`

	Type                  string
	maxConnectionLifetime time.Duration
	Initialized           bool
	db                    *sql.DB
	sync.Mutex
}

func (c *SQLConnectionProducer) Initialize(ctx context.Context, conf map[string]interface{}, verifyConnection bool) (saveConf map[string]interface{}, err error) {
	c.Lock()
	defer c.Unlock()

	err = mapstructure.WeakDecode(conf, c)
	if err != nil {
		return nil, err
	}

	connURL := c.ConnectionURL
	if len(connURL) == 0 {
		return nil, fmt.Errorf("connection_url cannot be empty")
	}

	if len(c.Username) != 0 && len(c.Password) != 0 {
		if !strings.Contains(connURL, "{{username}}") || !strings.Contains(connURL, "{{password}}") {
			return nil, fmt.Errorf("connection_url must be templated if username and password are provided")
		}

		dbutil.QueryHelper(connURL, map[string]string{
			"username": c.Username,
			"password": c.Password,
		})
	}

	if c.MaxOpenConnections == 0 {
		c.MaxOpenConnections = 2
	}

	if c.MaxIdleConnections == 0 {
		c.MaxIdleConnections = c.MaxOpenConnections
	}
	if c.MaxIdleConnections > c.MaxOpenConnections {
		c.MaxIdleConnections = c.MaxOpenConnections
	}
	if c.MaxConnectionLifetimeRaw == nil {
		c.MaxConnectionLifetimeRaw = "0s"
	}

	c.maxConnectionLifetime, err = parseutil.ParseDurationSecond(c.MaxConnectionLifetimeRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid max_connection_lifetime: %s", err)
	}

	// Set initialized to true at this point since all fields are set,
	// and the connection can be established at a later time.
	c.Initialized = true

	if verifyConnection {
		if _, err := c.Connection(ctx); err != nil {
			return nil, fmt.Errorf("error verifying connection: %s", err)
		}

		if err := c.db.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("error verifying connection: %s", err)
		}
	}

	return conf, nil
}

func (c *SQLConnectionProducer) Connection(ctx context.Context) (interface{}, error) {
	if !c.Initialized {
		return nil, ErrNotInitialized
	}

	// If we already have a DB, test it and return
	if c.db != nil {
		if err := c.db.PingContext(ctx); err == nil {
			return c.db, nil
		}
		// If the ping was unsuccessful, close it and ignore errors as we'll be
		// reestablishing anyways
		c.db.Close()
	}

	// For mssql backend, switch to sqlserver instead
	dbType := c.Type
	if c.Type == "mssql" {
		dbType = "sqlserver"
	}

	// Otherwise, attempt to make connection
	conn := c.ConnectionURL

	// Ensure timezone is set to UTC for all the conenctions
	if strings.HasPrefix(conn, "postgres://") || strings.HasPrefix(conn, "postgresql://") {
		if strings.Contains(conn, "?") {
			conn += "&timezone=utc"
		} else {
			conn += "?timezone=utc"
		}
	}

	var err error
	c.db, err = sql.Open(dbType, conn)
	if err != nil {
		return nil, err
	}

	// Set some connection pool settings. We don't need much of this,
	// since the request rate shouldn't be high.
	c.db.SetMaxOpenConns(c.MaxOpenConnections)
	c.db.SetMaxIdleConns(c.MaxIdleConnections)
	c.db.SetConnMaxLifetime(c.maxConnectionLifetime)

	return c.db, nil
}

// Close attempts to close the connection
func (c *SQLConnectionProducer) Close() error {
	// Grab the write lock
	c.Lock()
	defer c.Unlock()

	if c.db != nil {
		c.db.Close()
	}

	c.db = nil

	return nil
}
