package provisioning

import "iter"

type Endpoint interface {
	GetConnectionURL() string
	GetCertificate() string
	GetServerName() (string, error)
}

type Endpoints interface {
	GetEndpoints() iter.Seq[Endpoint]
}
