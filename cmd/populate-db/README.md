# populate-db

## Usage

Generate Test DB for a somewhat realistic, larger customer

* Number of clusters will be between 50-100.
* 2500 servers in total.
* Total instances of around 120k
* Around 500 networks each with around 50 ACLs and up to 10 peers. No usage of forwards, load balancers or zones. Up to 5 integrations per clusters.
* 5 profiles per project and 20 projects per cluster.
* 5 pools per cluster, no storage buckets and maybe 50 or so custom volumes per project.

```shell
go run ./cmd/populate-db --clusters 75 --images-min 5 --images-max 30 --instance-min 1200 --instance-max 2000 --network-acls-min 10000 --network-acls-max 25000 --network-forwards-min 0 --network-forwards-max 0 --network-integrations-min 4 --network-integrations-max 6 --network-load-balancers-min 0 --network-load-balancers-max 0 --network-peers-min 2500 --network-peers-max 5000 --network-zones-min 0 --network-zones-max 0 --networks-min 500 --networks-max 500 --profiles-min 75 --profiles-max 150 --projects-min 15 --projects-max 25 --servers-min 30 --servers-max 35 --storage-buckets-min 0 --storage-buckets-max 0 --storage-pools-min 4 --storage-pools-max 6 --storage-volumes-min 200 --storage-volumes-max 300
```
