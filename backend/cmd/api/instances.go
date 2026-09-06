package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	dbconnect "erent/internal/database/connect"
	upstreamdirectory "erent/internal/directory/upstream"
	applogger "erent/internal/logger"
	casbinrbac "erent/internal/middleware/casbin"
	jwtservice "erent/internal/middleware/jwt"
	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/modules/oauth/openai"
	"erent/internal/modules/user"
	"erent/internal/rpc/transport"
	"erent/internal/rpc/upstream"

	"github.com/casbin/casbin/v3"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type applicationInstances struct {
	sqlDatabase        *sql.DB
	redisClient        *redis.Client
	userRepository     *user.Repository
	casbinEnforcer     *casbin.SyncedEnforcer
	jwtManager         *jwtservice.JWTManager
	oaiOIDCAuth        *oidc.OIDCAuth
	upstreamConnection *grpc.ClientConn
	upstreamDirectory  *upstreamdirectory.Directory
}

func newApplicationLogger() (*slog.Logger, io.Closer, error) {
	return applogger.NewLogger(os.Getenv("LOG_FILE"))
}

func newApplicationInstances(configuration apiConfiguration) (_ *applicationInstances, resultErr error) {
	instances := &applicationInstances{}
	defer func() {
		if resultErr != nil {
			_ = instances.Close()
		}
	}()

	databaseContext, cancelDatabase := context.WithTimeout(
		context.Background(),
		configuration.runtime.DatabaseConnectTimeout,
	)
	database, sqlDatabase, err := dbconnect.Connect(databaseContext, configuration.runtime.DatabaseURL)
	cancelDatabase()
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	instances.sqlDatabase = sqlDatabase

	redisContext, cancelRedis := context.WithTimeout(
		context.Background(),
		configuration.runtime.DatabaseConnectTimeout,
	)
	redisClient, err := rdb.Connect(redisContext, configuration.runtime.Redis)
	cancelRedis()
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	instances.redisClient = redisClient
	instances.userRepository = user.NewRepository(database)

	if err := createBootstrapUser(context.Background(), configuration.runtime, instances.userRepository); err != nil {
		return nil, fmt.Errorf("create bootstrap user: %w", err)
	}

	instances.casbinEnforcer, err = casbinrbac.NewEnforcer(database)
	if err != nil {
		return nil, fmt.Errorf("initialize Casbin authorization: %w", err)
	}
	instances.jwtManager = jwtservice.NewJWTManager(configuration.runtime.JWT)

	if configuration.oai.Enabled() {
		credentials, err := transport.ClientCredentials(configuration.upstream.TLS)
		if err != nil {
			return nil, fmt.Errorf("initialize upstream transport: %w", err)
		}
		instances.upstreamConnection, err = grpc.NewClient(configuration.upstream.Target,
			grpc.WithTransportCredentials(credentials), grpc.WithDisableRetry())
		if err != nil {
			return nil, fmt.Errorf("create upstream gRPC client: %w", err)
		}
		instances.upstreamDirectory = upstreamdirectory.New(upstream.NewUpstreamServiceClient(instances.upstreamConnection), configuration.upstream.Timeout)
		oidcContext, cancelOIDC := context.WithTimeout(
			context.Background(),
			configuration.runtime.OIDCDiscoveryTimeout,
		)
		instances.oaiOIDCAuth, err = oidc.NewOIDCAuth(
			oidcContext,
			configuration.oai,
			openai.OaiAuthURLParams(),
			openai.OaiScopes(),
		)
		cancelOIDC()
		if err != nil {
			return nil, fmt.Errorf("initialize OIDC auth for oai: %w", err)
		}
	}

	return instances, nil
}

func (i *applicationInstances) Close() error {
	if i == nil {
		return nil
	}
	var closeErrors []error
	if i.upstreamConnection != nil {
		if err := i.upstreamConnection.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close upstream connection: %w", err))
		}
	}
	if i.redisClient != nil {
		if err := i.redisClient.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close redis: %w", err))
		}
	}
	if i.sqlDatabase != nil {
		if err := i.sqlDatabase.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close database: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}
