package db

import (
	"context"
	"time"

	"ollmo/ollmo/internal/config"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
)

// NewMilvus connects to a Milvus 2.5+ standalone/cluster.
func NewMilvus(cfg config.MilvusConfig) (client.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.NewClient(ctx, client.Config{
		Address: cfg.Addr(),
	})
}
