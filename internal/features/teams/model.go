package teams

import "aaa2ppp/teams-tasks/internal/model"

type Metric struct {
	Type   string                  `json:"type"`
	Detail string                  `json:"detail"`
	Value  model.Nullable[float64] `json:"value,omitzero" swaggertype:"number"`
}
