package provisioning

import "iter"

type Endpoint interface {
	GetConnectionURL() string
	GetCertificate() string
}

type Endpoints interface {
	GetEndpoints() iter.Seq[Endpoint]
}
