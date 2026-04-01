package content

import "time"

type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeRichText FieldType = "richtext"
	FieldTypeNumber   FieldType = "number"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDate     FieldType = "date"
	FieldTypeMedia    FieldType = "media"
	FieldTypeRelation FieldType = "relation"
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
