package config

// GRPCConfig holds gRPC server configuration.
type GRPCConfig struct {
	GRPCPort string `env:"MONO_GRPC_PORT" default:"8080"`
	GRPCHost string `env:"MONO_GRPC_HOST" default:"localhost"`

	GRPCKeepaliveTime                           int  `env:"MONO_GRPC_KEEPALIVE_TIME" default:"300"`
	GRPCKeepaliveTimeout                        int  `env:"MONO_GRPC_KEEPALIVE_TIMEOUT" default:"20"`
	GRPCMaxConnectionIdle                       int  `env:"MONO_GRPC_MAX_CONNECTION_IDLE" default:"900"`
	GRPCMaxConnectionAge                        int  `env:"MONO_GRPC_MAX_CONNECTION_AGE" default:"1800"`
	GRPCMaxConnectionAgeGrace                   int  `env:"MONO_GRPC_MAX_CONNECTION_AGE_GRACE" default:"5"`
	GRPCConnectionTimeout                       int  `env:"MONO_GRPC_CONNECTION_TIMEOUT" default:"120"`
	GRPCKeepaliveEnforcementMinTime             int  `env:"MONO_GRPC_KEEPALIVE_ENFORCEMENT_MIN_TIME" default:"5"`
	GRPCKeepaliveEnforcementPermitWithoutStream bool `env:"MONO_GRPC_KEEPALIVE_ENFORCEMENT_PERMIT_WITHOUT_STREAM" default:"false"`
}
