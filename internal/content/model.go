package content

import "time"

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
}

type ContentType struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Fields      []Field   `json:"fields"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Entry struct {
	ID            int64          `json:"id"`
	ContentTypeID int64          `json:"content_type_id"`
	Data          map[string]any `json:"data"`
	PublishedAt   *time.Time     `json:"published_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
