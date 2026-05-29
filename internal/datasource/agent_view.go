package datasource

type AgentView struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        DataSourceType `json:"type"`
	Host        string         `json:"host,omitempty"`
	Port        int            `json:"port,omitempty"`
	Username    string         `json:"username,omitempty"`
	Password    string         `json:"password,omitempty"`
	Database    string         `json:"database,omitempty"`
	AuthSource  string         `json:"authSource,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Dialect     string         `json:"dialect,omitempty"`
}

func ToAgentView(ds DataSource) AgentView {
	ds = RedactDatasource(ds)
	return AgentView{
		ID:          ds.ID,
		Name:        ds.Name,
		Type:        ds.Type,
		Host:        ds.Host,
		Port:        ds.Port,
		Username:    ds.Username,
		Password:    ds.Password,
		Database:    ds.Database,
		AuthSource:  ds.AuthSource,
		Options:     ds.Options,
		Environment: ds.Environment(),
		Dialect:     ds.QueryDialect(),
	}
}

func ToAgentViews(items []DataSource) []AgentView {
	out := make([]AgentView, 0, len(items))
	for _, item := range items {
		out = append(out, ToAgentView(item))
	}
	return out
}
