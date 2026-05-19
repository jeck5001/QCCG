package common

// QoderModel 是返回给前端的精简模型条目（仅保留下拉选择必要字段）
type QoderModel struct {
	Key            string `json:"key"`
	DisplayName    string `json:"display_name"`
	Enable         bool   `json:"enable"`
	IsDefault      bool   `json:"is_default"`
	MaxInputTokens int    `json:"max_input_tokens,omitempty"`
}