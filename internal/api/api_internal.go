package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/dump"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

type internalHandler struct {
	db dbdriver.DBTX
}

func registerInternalHandler(router Router, authorizer *authz.Authorizer, db dbdriver.DBTX) {
	handler := &internalHandler{
		db: db,
	}

	router.HandleFunc("GET /sql", response.With(handler.sqlGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /sql", response.With(handler.sqlPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

func (i *internalHandler) sqlGet(r *http.Request) response.Response {
	dumpFormValue := r.FormValue("dump")
	dumpInt, err := strconv.Atoi(dumpFormValue)
	if err != nil {
		dumpInt = 0
	}

	dumpOption := dump.Option(dumpInt)

	var dumpResult string
	err = transaction.Do(r.Context(), func(ctx context.Context) error {
		var err error

		switch dumpOption {
		case dump.OptionDefault:
			dumpResult, err = dumpSchema(ctx, i.db, false)

		case dump.OptionSchema:
			dumpResult, err = dumpSchema(ctx, i.db, true)

		case dump.OptionTables:
			dumpResult, err = dumpTables(ctx, i.db)

		default:
			return fmt.Errorf("Failed to perform dump due to missing dump option")
		}

		return err
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed dump database: %w", err))
	}

	return response.SyncResponse(true, dump.SQLDump{Text: dumpResult})
}

func (i *internalHandler) sqlPost(r *http.Request) response.Response {
	ctx := r.Context()

	req := &dump.SQLQuery{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	if req.Query == "" {
		return response.BadRequest(errors.New("No query provided"))
	}

	batch := dump.SQLBatch{}
	for query := range strings.SplitSeq(req.Query, ";") {
		query = strings.TrimLeft(query, " ")

		if query == "" {
			continue
		}

		result := dump.SQLResult{}

		err := transaction.Do(ctx, func(ctx context.Context) error {
			if strings.HasPrefix(strings.ToUpper(query), "SELECT") {
				err = i.internalSQLSelect(ctx, query, &result)
			} else {
				err = i.internalSQLExec(ctx, query, &result)
			}

			if err != nil {
				return err
			}

			batch.Results = append(batch.Results, result)

			return nil
		})
		if err != nil {
			return response.SmartError(err)
		}
	}

	return response.SyncResponse(true, batch)
}
