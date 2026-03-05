package provisioning

import "iter"

type Endpoint interface {
	GetConnectionURL() string
	GetCertificate() string
	GetServerName() (string, error)
	GetName() string
}

type Endpoints interface {
	GetEndpoints() iter.Seq[Endpoint]
}
