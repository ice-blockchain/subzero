// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func RunPostgresContainer(ctx context.Context, image, databaseName, user, password string, timeout time.Duration) (container *postgres.PostgresContainer, testContainerPort string) {
	fmt.Println("creating postgres container...")
	container, err := postgres.Run(ctx,
		image,
		postgres.WithDatabase(databaseName),
		postgres.WithUsername(user),
		postgres.WithPassword(password),
		// postgres.WithSSLCert(),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		panic(err)
	}
	state, err := container.State(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to get container state:%v", err))
	}
	if !state.Running {
		panic("container is not running")
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(fmt.Errorf("failed to get mapped port: %w", err))
	}
	testContainerPort = port.Port()
	fmt.Println("PostgreSQL container started on port:", testContainerPort)

	return container, testContainerPort

}
