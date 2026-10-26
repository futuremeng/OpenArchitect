package model

type RelType string

const (
	RelCalls     RelType = "calls"
	RelDependsOn RelType = "depends_on"
	RelContains  RelType = "contains"
)

// Relationship carries Weight instead of an ID: the (from,to,type) triple is
// unique after de-duplication (spec §6.4), so a surrogate key is pure cost.
type Relationship struct {
	FromID string
	ToID   string
	Type   RelType
	Weight int
}
