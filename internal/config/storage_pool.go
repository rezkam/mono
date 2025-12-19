package config

// StoragePoolConfig holds storage connection pool configuration.
type StoragePoolConfig struct {
	DBMaxOpenConns    int `env:"MONO_DB_MAX_OPEN_CONNS" default:"25"`
	DBMaxIdleConns    int `env:"MONO_DB_MAX_IDLE_CONNS" default:"5"`
	DBConnMaxLifetime int `env:"MONO_DB_CONN_MAX_LIFETIME" default:"300"`
	DBConnMaxIdleTime int `env:"MONO_DB_CONN_MAX_IDLE_TIME" default:"60"`
}
