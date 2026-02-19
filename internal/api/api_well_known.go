package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type wellKnownHandler struct{}

func registerWellKnownHandler(router Router) {
	handler := &wellKnownHandler{}

	router.HandleFunc("GET /.well-known/acme-challenge/{token}", response.With(handler.acmeProvideChallenge))
}

func (w *wellKnownHandler) acmeProvideChallenge(r *http.Request) response.Response {
	acmeCfg := config.GetSecurity().ACME
	if acmeCfg.Challenge != api.ACMEChallengeHTTP {
		return response.NotFound(nil)
	}

	if acmeCfg.Domain == "" {
		return response.SmartError(errors.New("ACME domain is not configured"))
	}

	httpChallengeAddr := acmeCfg.Address
	if strings.HasPrefix(httpChallengeAddr, ":") {
		httpChallengeAddr = "127.0.0.1" + httpChallengeAddr
	}

	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("tcp", httpChallengeAddr)
			},
		},
	}

	req, err := http.NewRequest("GET", "http://"+acmeCfg.Domain+r.URL.String(), nil)
	if err != nil {
		return response.InternalError(err)
	}

	req.Header = r.Header
	resp, err := client.Do(req)
	if err != nil {
		return response.InternalError(err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response.InternalError(err)
	}

	return response.ManualResponse(func(w http.ResponseWriter) error {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(resp.StatusCode)

		_, err = w.Write(body)
		if err != nil {
			return err
		}

		return nil
	})
}
