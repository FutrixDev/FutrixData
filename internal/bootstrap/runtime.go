package bootstrap

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/appdata"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourcesecrets"
	"futrixdata/platform/internal/history"
	"futrixdata/platform/internal/redisproto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/secrets"
	"futrixdata/platform/internal/securefile"
)

type Config struct {
	DataPath               string
	AuxiliaryLoadMode      AuxiliaryLoadMode
	DatasourceLoadPolicy   LoadPolicy
	AIConfigLoadPolicy     LoadPolicy
	RedisDocsLoadPolicy    LoadPolicy
	EntityCacheLoadPolicy  LoadPolicy
	HistoryLoadPolicy      LoadPolicy
	RedisProtoLoadPolicy   LoadPolicy
	SecretConfigLoadPolicy LoadPolicy
}

type AuxiliaryLoadMode string

type LoadPolicy string

const (
	AuxiliaryLoadStrict     AuxiliaryLoadMode = ""
	AuxiliaryLoadBestEffort AuxiliaryLoadMode = "best-effort"

	LoadPolicyStrict     LoadPolicy = ""
	LoadPolicyBestEffort LoadPolicy = "best-effort"
)

type Runtime struct {
	DataPath          string
	Store             *datasource.Store
	AIConfigStore     *aiconfig.Store
	Manager           *console.Manager
	RiskEngine        *riskengine.Engine
	RiskGuard         *riskengine.Guard
	RiskStore         *riskengine.Store
	RedisDocs         *console.RedisCommandDocsStore
	EntityCache       *console.EntitySchemaCacheStore
	HistoryStore      *history.Store
	RedisProtoStore   *redisproto.Store
	SecretConfigs     *secrets.ProviderConfigStore
	SecretRegistry    *secrets.Registry
	DatasourceSecrets *datasourcesecrets.Manager
}

func ResolveDataPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return trimmed
	}
	return appdata.DevDataPath("FutrixData")
}

func AIConfigPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "aiconfigs.json")
}

func RedisCommandDocsPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "redis-command-docs.json")
}

func EntitySchemaCachePath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "entity-schema-cache.json")
}

func HistoryPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "history", "console-history.json")
}

func RedisProtobufPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "redis-protobuf.json")
}

func AgentIdentityPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "history", "agent-identities.json")
}

func AgentAuditPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "history", "agent-audit.jsonl")
}

// SchemaPrivacyAuditPath is the jsonl log of schema-to-LLM events. Lives next
// to the agent audit log so users investigating data egress find them in one
// place, but the format and retention story are independent.
func SchemaPrivacyAuditPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "history", "schema-llm-audit.jsonl")
}

func SensitivityStorePath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "sensitivity-classifications.json")
}

func RiskRulesPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "risk-rules")
}

func SecretProviderConfigPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "secret-providers.json")
}

func SchemaKnowledgeRoot(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), ".data", "user-kb", "customer-knowledge-base")
}

func UserKBRoot(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), ".data", "user-kb")
}

func NewRuntime(cfg Config) (*Runtime, error) {
	dataPath := ResolveDataPath(cfg.DataPath)

	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		if cfg.DatasourceLoadPolicy != LoadPolicyBestEffort || !errors.Is(err, securefile.ErrKeyUnavailable) {
			return nil, fmt.Errorf("load datasources: %w", err)
		}
	}

	aiConfigStore := aiconfig.NewStore(AIConfigPath(dataPath))
	if err := loadAuxiliaryStore(aiConfigStore.Load, cfg.resolvePolicy(cfg.AIConfigLoadPolicy), "ai configs"); err != nil {
		return nil, err
	}

	redisDocs := console.NewRedisCommandDocsStore(RedisCommandDocsPath(dataPath))
	if err := loadAuxiliaryStore(redisDocs.Load, cfg.resolvePolicy(cfg.RedisDocsLoadPolicy), "redis command docs"); err != nil {
		return nil, err
	}

	entityCache := console.NewEntitySchemaCacheStore(EntitySchemaCachePath(dataPath))
	if err := loadAuxiliaryStore(entityCache.Load, cfg.resolvePolicy(cfg.EntityCacheLoadPolicy), "entity schema cache"); err != nil {
		return nil, err
	}

	historyStore := history.NewStore(HistoryPath(dataPath))
	if err := loadAuxiliaryStore(historyStore.Load, cfg.resolvePolicy(cfg.HistoryLoadPolicy), "history"); err != nil {
		return nil, err
	}

	redisProtoStore := redisproto.NewStore(RedisProtobufPath(dataPath))
	if err := loadAuxiliaryStore(redisProtoStore.Load, cfg.resolvePolicy(cfg.RedisProtoLoadPolicy), "redis protobuf schemas"); err != nil {
		return nil, err
	}

	manager := console.NewManager()
	manager.Register(datasource.TypeMySQL, console.NewMySQLAdapter())
	manager.Register(datasource.TypePostgreSQL, console.NewPostgresAdapter())
	manager.Register(datasource.TypeMongoDB, console.NewMongoAdapter())
	manager.Register(datasource.TypeRedis, console.NewRedisAdapter())
	manager.Register(datasource.TypeElasticsearch, console.NewElasticsearchAdapter())
	manager.Register(datasource.TypeChromaDB, console.NewChromaDBAdapter())
	manager.Register(datasource.TypeDynamoDB, console.NewDynamoDBAdapter())
	manager.Register(datasource.TypeD1, console.NewD1Adapter())

	secretConfigs := secrets.NewProviderConfigStore(SecretProviderConfigPath(dataPath))
	secretPolicy := cfg.resolvePolicy(cfg.SecretConfigLoadPolicy)
	if err := loadAuxiliaryStore(secretConfigs.Load, secretPolicy, "secret provider configs"); err != nil {
		return nil, err
	}
	secretRegistry, err := secrets.NewRegistry(secretConfigs.List())
	if err != nil {
		if secretPolicy != LoadPolicyBestEffort {
			return nil, fmt.Errorf("load secret providers: %w", err)
		}
		// Best-effort callers (daemon/CLI/HTTP) treat the provider config as
		// optional: a semantically invalid entry (unsupported type, missing Vault
		// address) must not take down startup. Fall back to an empty registry —
		// datasources without refs keep working, and the daemon can Reload once the
		// config becomes valid/decryptable, mirroring the load-failure path above.
		secretRegistry, err = secrets.NewRegistry(nil)
		if err != nil {
			return nil, fmt.Errorf("load secret providers: %w", err)
		}
	}
	datasourceSecrets := datasourcesecrets.NewManager(secretRegistry)
	manager.SetDatasourceSecretResolver(datasourceSecrets)

	riskEng := riskengine.NewEngine()
	riskStore := riskengine.NewStore(RiskRulesPath(dataPath))
	_ = riskStore.Load() // best-effort; missing dir is fine
	riskEng.ReloadFromStore(riskStore)
	guard := riskengine.NewGuard(riskEng)
	guard.SetProbeProvider(manager)
	manager.SetInterceptor(guard)

	return &Runtime{
		DataPath:          dataPath,
		Store:             store,
		AIConfigStore:     aiConfigStore,
		Manager:           manager,
		RiskEngine:        riskEng,
		RiskGuard:         guard,
		RiskStore:         riskStore,
		RedisDocs:         redisDocs,
		EntityCache:       entityCache,
		HistoryStore:      historyStore,
		RedisProtoStore:   redisProtoStore,
		SecretConfigs:     secretConfigs,
		SecretRegistry:    secretRegistry,
		DatasourceSecrets: datasourceSecrets,
	}, nil
}

func (cfg Config) resolvePolicy(explicit LoadPolicy) LoadPolicy {
	if explicit != LoadPolicyStrict {
		return explicit
	}
	if cfg.AuxiliaryLoadMode == AuxiliaryLoadBestEffort {
		return LoadPolicyBestEffort
	}
	return LoadPolicyStrict
}

func loadAuxiliaryStore(load func() error, policy LoadPolicy, label string) error {
	if err := load(); err != nil {
		if policy == LoadPolicyBestEffort {
			return nil
		}
		return fmt.Errorf("load %s: %w", label, err)
	}
	return nil
}
