package simplestreams

import "github.com/FuturFusion/operations-center/shared/api"

// type (
// 	// Stream is a simplestream stream base struct.
// 	//
// 	// swagger:model SimplestreamsStream
// 	Stream      = simplestreams.Stream
// 	StreamIndex = simplestreams.StreamIndex

// 	// Products is a simplestream products base struct.
// 	//
// 	// swagger:model SimplestreamsProducts
// 	Products           = simplestreams.Products
// 	Product            = simplestreams.Product
// 	ProductVersion     = simplestreams.ProductVersion
// 	ProductVersionItem = simplestreams.ProductVersionItem
// )

// Stream represents the base structure of index.json.
//
// swagger:model SimplestreamsStream
type Stream struct {
	Index   map[string]StreamIndex `json:"index"`
	Updated string                 `json:"updated,omitempty"`
	Format  string                 `json:"format"`
}

// StreamIndex represents the Index entry inside index.json.
//
// swagger:model SimplestreamsStreamIndex
type StreamIndex struct {
	DataType string   `json:"datatype"`
	Path     string   `json:"path"`
	Updated  string   `json:"updated,omitempty"`
	Products []string `json:"products"`
	Format   string   `json:"format,omitempty"`
}

// Products represents the base of download.json.
//
// swagger:model SimplestreamsProducts
type Products struct {
	ContentID string             `json:"content_id"`
	DataType  string             `json:"datatype"`
	Format    string             `json:"format"`
	License   string             `json:"license,omitempty"`
	Products  map[string]Product `json:"products"`
	Updated   string             `json:"updated,omitempty"`
}

// Product represents a single product inside download.json.
//
// swagger:model SimplestreamsProduct
type Product struct {
	Aliases         string                           `json:"aliases"`
	Architecture    string                           `json:"arch"`
	OperatingSystem string                           `json:"os"`
	Requirements    map[string]string                `json:"requirements,omitempty"`
	Release         string                           `json:"release"`
	ReleaseCodename string                           `json:"release_codename,omitempty"`
	ReleaseTitle    string                           `json:"release_title"`
	Supported       bool                             `json:"supported,omitempty"`
	SupportedEOL    string                           `json:"support_eol,omitempty"`
	Version         string                           `json:"version,omitempty"`
	Versions        map[string]api.IncusImageVersion `json:"versions"`

	// Non-standard fields (only used on some image servers).
	Variant string `json:"variant,omitempty"`
}
