package userkb

type CategoryScope string

const (
	ScopeAll        CategoryScope = "all"
	ScopeDatasource CategoryScope = "datasource"
)

type ParseStatus string

const (
	ParseQueued ParseStatus = "queued"
	ParseOK     ParseStatus = "ok"
	ParseFailed ParseStatus = "failed"
)

type SummaryStatus string

const (
	SummaryQueued        SummaryStatus = "queued"
	SummaryOK            SummaryStatus = "ok"
	SummaryFailed        SummaryStatus = "failed"
	SummaryNeedsProvider SummaryStatus = "needs_provider"
	SummarySkipped       SummaryStatus = "skipped"
)

type StoreState struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
	Files      []File     `json:"files"`
}

type Category struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Scope         CategoryScope `json:"scope"`
	DatasourceIDs []string      `json:"datasourceIds,omitempty"`
	CreatedAt     int64         `json:"createdAt"`
	UpdatedAt     int64         `json:"updatedAt"`
}

type File struct {
	ID            string        `json:"id"`
	CategoryID    string        `json:"categoryId"`
	OriginalName  string        `json:"originalName"`
	Ext           string        `json:"ext"`
	Size          int64         `json:"size"`
	UploadPath    string        `json:"uploadPath"`
	ParsedPath    string        `json:"parsedPath"`
	ParseStatus   ParseStatus   `json:"parseStatus"`
	ParseError    string        `json:"parseError,omitempty"`
	SummaryStatus SummaryStatus `json:"summaryStatus"`
	SummaryError  string        `json:"summaryError,omitempty"`
	Note          string        `json:"note,omitempty"`
	AISummary     string        `json:"aiSummary,omitempty"`
	Keywords      []string      `json:"keywords,omitempty"`
	CreatedAt     int64         `json:"createdAt"`
	UpdatedAt     int64         `json:"updatedAt"`
}

type CategoryCreateInput struct {
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Scope         CategoryScope `json:"scope"`
	DatasourceIDs []string      `json:"datasourceIds,omitempty"`
}

type CategoryUpdateInput struct {
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Scope         CategoryScope `json:"scope"`
	DatasourceIDs []string      `json:"datasourceIds,omitempty"`
}

type UploadFileInput struct {
	Name   string `json:"name"`
	Base64 string `json:"base64"`
}

type ViewState struct {
	State             StoreState `json:"state"`
	AIProviderReady   bool       `json:"aiProviderReady"`
	AIProviderMessage string     `json:"aiProviderMessage,omitempty"`
}
