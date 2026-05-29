package console

type redisMode string

const (
	redisModeStandalone redisMode = "standalone"
	redisModeCluster    redisMode = "cluster"
)

type redisConnInfo struct {
	Mode  redisMode
	Nodes []string
}

type redisClusterNode struct {
	ID   string
	Addr string
	Role string
}
