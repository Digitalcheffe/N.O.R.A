package models

// ComponentLink represents a parent-child relationship between any two entities
// in NORA. Each child has exactly one parent.
//
// parent_type / child_type match the type strings used in infrastructure_components
// (e.g. "proxmox_node", "vm_linux", "docker_engine") plus the entity table names
// for standalone entities ("docker_engine", "app", "container").
type ComponentLink struct {
	ParentType string `db:"parent_type" json:"parent_type"`
	ParentID   string `db:"parent_id"   json:"parent_id"`
	ChildType  string `db:"child_type"  json:"child_type"`
	ChildID    string `db:"child_id"    json:"child_id"`
	CreatedAt  string `db:"created_at"  json:"created_at"`
}
