package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/generator/metadata"
	sqlitegen "github.com/go-jet/jet/v2/generator/sqlite"
	"github.com/go-jet/jet/v2/generator/template"
	"github.com/go-jet/jet/v2/sqlite"
	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func main() {
	tmpDir, err := os.MkdirTemp("", "jet-generate-*")
	if err != nil {
		log.Panic(err)
	}

	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			log.Panic(err)
		}
	}()

	db, err := dbdriver.Open(tmpDir)
	if err != nil {
		log.Panic(err)
	}

	defer func() {
		err = db.Close()
		if err != nil {
			log.Panic(err)
		}
	}()

	_, err = dbschema.Ensure(context.Background(), db, tmpDir)
	if err != nil {
		log.Panic(err)
	}

	dbFile := filepath.Join(tmpDir, "local.db")
	err = sqlitegen.GenerateDSN(dbFile, "../.gen", genTemplate([]string{"schema"}, nil, nil))
	if err != nil {
		log.Panic(err)
	}
}

func genTemplate(ignoreTables []string, ignoreViews []string, ignoreEnums []string) template.Template {
	shouldSkipTable := func(table metadata.Table) bool {
		return slices.Contains(ignoreTables, strings.ToLower(table.Name))
	}

	shouldSkipView := func(view metadata.Table) bool {
		return slices.Contains(ignoreViews, strings.ToLower(view.Name))
	}

	shouldSkipEnum := func(enum metadata.Enum) bool {
		return slices.Contains(ignoreEnums, strings.ToLower(enum.Name))
	}

	return template.Default(sqlite.Dialect).
		UseSchema(func(schema metadata.Schema) template.Schema {
			return template.DefaultSchema(schema).
				UseModel(template.DefaultModel().UsePath("model").
					UseTable(func(table metadata.Table) template.TableModel {
						if shouldSkipTable(table) {
							return template.TableModel{Skip: true}
						}

						return template.DefaultTableModel(table).UseField(func(column metadata.Column) template.TableModelField {
							defaultTableModelField := template.DefaultTableModelField(column)

							if table.Name == "tokens" {
								if column.Name == "uuid" {
									defaultTableModelField.Type = template.NewType(uuid.UUID{})
								}

								if column.Name == "expire_at" {
									defaultTableModelField.Type = template.NewType(time.Time{})
								}

								if column.Name == "uses_remaining" {
									defaultTableModelField.Type = template.NewType(int(0))
								}
							}

							return defaultTableModelField
						})
					}).
					UseView(func(view metadata.Table) template.ViewModel {
						if shouldSkipView(view) {
							return template.ViewModel{Skip: true}
						}

						return template.DefaultViewModel(view)
					}).
					UseEnum(func(enum metadata.Enum) template.EnumModel {
						if shouldSkipEnum(enum) {
							return template.EnumModel{Skip: true}
						}

						return template.DefaultEnumModel(enum)
					}),
				).
				UseSQLBuilder(template.DefaultSQLBuilder().
					UseTable(func(table metadata.Table) template.TableSQLBuilder {
						if shouldSkipTable(table) {
							return template.TableSQLBuilder{Skip: true}
						}

						return template.DefaultTableSQLBuilder(table).UsePath("table")
					}).
					UseView(func(table metadata.Table) template.ViewSQLBuilder {
						if shouldSkipView(table) {
							return template.ViewSQLBuilder{Skip: true}
						}

						return template.DefaultViewSQLBuilder(table).UsePath("view")
					}).
					UseEnum(func(enum metadata.Enum) template.EnumSQLBuilder {
						if shouldSkipEnum(enum) {
							return template.EnumSQLBuilder{Skip: true}
						}

						return template.DefaultEnumSQLBuilder(enum).UsePath("enum")
					}),
				)
		})
}
