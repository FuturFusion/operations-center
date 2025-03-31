package seed

import (
	"math/rand/v2"

	"github.com/brianvoe/gofakeit/v7"
	incusapi "github.com/lxc/incus/v6/shared/api"
)

func randBetween(from, to int) int {
	if from == to {
		return from
	}

	if to < from {
		to, from = from, to
	}

	if from < 0 {
		from = 0
	}

	if to < 0 {
		to = 10
	}

	return rand.IntN(to-from) + from
}

func randomArchitecture() string {
	return gofakeit.RandomString([]string{
		"i686", "x86_64", "armv6l", "armv7l", "armv8l", "aarch64", "ppc", "ppc64", "ppc64le", "s390x", "mips", "mips64", "riscv32", "riscv64", "loongarch64",
	})
}

func randomType() string {
	return gofakeit.RandomString([]string{"container", "virtual-machine"})
}

func randomNetworkType() string {
	return gofakeit.RandomString([]string{"bridge", "physical", "macvlan", "loopback", "ovn"})
}

func randomStatus() string {
	return gofakeit.RandomString([]string{"Pending", "Created", "Errored", "Unknown"})
}

func randomSelection(list []string) []string {
	ret := []string{}

	for _, item := range list {
		if gofakeit.Bool() {
			ret = append(ret, item)
		}
	}

	return ret
}

func randomStoragePoolDriver() string {
	return gofakeit.RandomString([]string{"dir", "zfs", "ceph", "lvmcluster"})
}

func randomInstanceState() string {
	return gofakeit.RandomString([]string{"Running", "Stopped", "Frozen", "Error"})
}

var instanceStates = map[string]int{
	"Running": 101,
	"Stopped": 102,
	"Frozen":  103,
	"Error":   104,
}

func instanceStateCode(state string) incusapi.StatusCode {
	s, ok := instanceStates[state]
	if !ok {
		return incusapi.StatusCode(104)
	}

	return incusapi.StatusCode(s)
}
