package content

import (
	"encoding/json"
	"time"
)

type FieldType string

const (
	FieldTypeText        FieldType = "text"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeRichText    FieldType = "richtext"    // Markdown
	FieldTypeBlocks      FieldType = "blocks"      // Rich text JSON blocks
	FieldTypeJSON        FieldType = "json"
	FieldTypeNumber      FieldType = "number"
	FieldTypeEmail       FieldType = "email"
	FieldTypeDate        FieldType = "date"
	FieldTypePassword    FieldType = "password"
	FieldTypeMedia       FieldType = "media"
	FieldTypeEnumeration FieldType = "enumeration"
	FieldTypeRelation    FieldType = "relation"
	FieldTypeUID         FieldType = "uid"
)

type Field struct {
	ID            int64     `json:"id"`
	ContentTypeID int64     `json:"content_type_id"`
	Name          string    `json:"name"`
	Type          FieldType `json:"type"`
	Required      bool      `json:"required"`
	Order         int       `json:"order"`
	IsBuiltin     bool      `json:"is_builtin,omitempty"` // true for User CT native columns
}

// userBuiltinFields returns the fixed column set exposed for the User CT.
// password_hash and totp_secret are intentionally omitted.
// IDs are negative to avoid collision with real field rows.
func userBuiltinFields(ctID int64) []Field {
	return []Field{
		{ID: -1, ContentTypeID: ctID, Name: "name",     Type: FieldTypeText,     Required: true,  Order: 0, IsBuiltin: true},
		{ID: -2, ContentTypeID: ctID, Name: "email",    Type: FieldTypeEmail,    Required: true,  Order: 1, IsBuiltin: true},
		{ID: -3, ContentTypeID: ctID, Name: "password", Type: FieldTypePassword, Required: false, Order: 2, IsBuiltin: true},
		{ID: -4, ContentTypeID: ctID, Name: "is_admin", Type: FieldTypeBoolean,  Required: false, Order: 3, IsBuiltin: true},
	}
}

// ── Endpoint configuration ───────────────────────────────────────────────────

// MethodFeatures holds opt-in capabilities for each HTTP method on a public
// endpoint.  All fields default to false; only explicitly enabled features
// are active.
type MethodFeatures struct {
	Pagination     bool `json:"pagination,omitempty"`
	Filters        bool `json:"filters,omitempty"`
	Sort           bool `json:"sort,omitempty"`
	Search         bool `json:"search,omitempty"`
	FieldSelection bool `json:"field_selection,omitempty"`
	Sanitize       bool `json:"sanitize,omitempty"`
	Partial        bool `json:"partial,omitempty"` // PATCH support for update
}

// EndpointConfig drives the /v1/* public route behaviour for a content type.
//
//   - Public  true  → no API key required
//   - Methods       → map of "find"|"findOne"|"create"|"update"|"delete" → bool
//   - Features      → per-method feature toggles
type EndpointConfig struct {
	Public   bool                      `json:"public"`
	Methods  map[string]bool           `json:"methods"`
	Features map[string]MethodFeatures `json:"features"`
}

// DefaultEndpointConfig returns a safe starting configuration (private, all
// CRUD enabled, only pagination active).
func DefaultEndpointConfig() EndpointConfig {
	return EndpointConfig{
		Public: false,
		Methods: map[string]bool{
			"find":    true,
			"findOne": true,
			"create":  true,
			"update":  true,
			"delete":  true,
		},
		Features: map[string]MethodFeatures{
			"find":    {Pagination: true},
			"findOne": {},
			"create":  {Sanitize: true},
			"update":  {Sanitize: true, Partial: true},
			"delete":  {},
		},
	}
}

// MarshalJSON ensures non-nil maps are always serialised.
func (e EndpointConfig) MarshalJSON() ([]byte, error) {
	if e.Methods == nil {
		e.Methods = map[string]bool{}
	}
	if e.Features == nil {
		e.Features = map[string]MethodFeatures{}
	}
	type alias EndpointConfig
	return json.Marshal(alias(e))
}

// ── Content type ─────────────────────────────────────────────────────────────

type ContentType struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Description    string         `json:"description"`
	IsSystem       bool           `json:"is_system"`
	EndpointConfig EndpointConfig `json:"endpoint_config"`
	Fields         []Field        `json:"fields"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ── Entry ─────────────────────────────────────────────────────────────────────

type Entry struct {
	ID            int64          `json:"id"`
	ContentTypeID int64          `json:"content_type_id"`
	Data          map[string]any `json:"data"`
	PublishedAt   *time.Time     `json:"published_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
