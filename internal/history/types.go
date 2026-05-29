package history

type Entry struct {
	ID             string   `json:"id"`
	Statement      string   `json:"statement"`
	ExecutedAt     string   `json:"executedAt"`
	DatasourceID   string   `json:"datasourceId"`
	DatasourceName string   `json:"datasourceName"`
	DatasourceType string   `json:"datasourceType"`
	Database       string   `json:"database"`
	Targets        []string `json:"targets"`
	Tags           []string `json:"tags"`
}

type AppendInput struct {
	DatasourceID   string
	DatasourceName string
	DatasourceType string
	Database       string
	Statement      string
	Targets        []string
}

type Filter struct {
	DatasourceID string
	Target       string
	Database     string
	Keyword      string
	Limit        int
}
