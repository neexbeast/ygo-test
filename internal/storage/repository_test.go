package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neexbeast/ygo-test/internal/destination"
	"github.com/neexbeast/ygo-test/internal/storage"
)

// ---- mock Querier ----

type mockQuerier struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFn(ctx, sql, args...)
}
func (m *mockQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return m.queryFn(ctx, sql, args...)
}
func (m *mockQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return m.execFn(ctx, sql, args...)
}

// ---- mock pgx.Row ----

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f *fakeRow) Scan(dest ...any) error { return f.scanFn(dest...) }

// ---- mock pgx.Rows ----

type fakeRows struct {
	rows    [][]any
	idx     int
	rowErr  error
	scanErr error
}

func (f *fakeRows) Next() bool                                   { f.idx++; return f.idx <= len(f.rows) }
func (f *fakeRows) Err() error                                   { return f.rowErr }
func (f *fakeRows) Close()                                       {}
func (f *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (f *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (f *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (f *fakeRows) RawValues() [][]byte                          { return nil }
func (f *fakeRows) Conn() *pgx.Conn                              { return nil }

func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	row := f.rows[f.idx-1]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch v := d.(type) {
		case *int:
			*v = row[i].(int)
		case *string:
			*v = row[i].(string)
		case *[]byte:
			*v = row[i].([]byte)
		case **time.Time:
			if row[i] == nil {
				*v = nil
			} else {
				t := row[i].(time.Time)
				*v = &t
			}
		case *time.Time:
			*v = row[i].(time.Time)
		}
	}
	return nil
}

// ---- mock MigrationPool ----

type mockMigrationPool struct {
	beginFn func(ctx context.Context) (pgx.Tx, error)
}

func (m *mockMigrationPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return m.beginFn(ctx)
}

// mockTx is a minimal pgx.Tx implementation for testing migrations.
type mockTx struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	commitFn   func(ctx context.Context) error
	rollbackFn func(ctx context.Context) error
}

func (t *mockTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.execFn(ctx, sql, args...)
}
func (t *mockTx) Commit(ctx context.Context) error   { return t.commitFn(ctx) }
func (t *mockTx) Rollback(ctx context.Context) error { return t.rollbackFn(ctx) }

// pgx.Tx has many more methods — stub them all out.
func (t *mockTx) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (t *mockTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *mockTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (t *mockTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (t *mockTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *mockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (t *mockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}
func (t *mockTx) Conn() *pgx.Conn { return nil }

// ---- helpers ----

func marshalData(t *testing.T, data destination.DestinationData) []byte {
	t.Helper()
	b, err := json.Marshal(data)
	require.NoError(t, err)
	return b
}

func writeSQLFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// ---- GetDestination tests ----

func TestGetDestination_Found(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	data := destination.DestinationData{
		Weather: &destination.WeatherData{Temperature: 22.5, Description: "clear sky"},
	}
	dataJSON := marshalData(t, data)

	q := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &fakeRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				*dest[1].(*string) = "Paris"
				*dest[2].(*string) = "France"
				*dest[3].(*[]byte) = dataJSON
				*dest[4].(**time.Time) = &now
				*dest[5].(*time.Time) = now
				*dest[6].(*time.Time) = now
				return nil
			}}
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	dest, err := repo.GetDestination(context.Background(), "Paris")
	require.NoError(t, err)
	require.NotNil(t, dest)
	assert.Equal(t, "Paris", dest.City)
	assert.Equal(t, 22.5, dest.Data.Weather.Temperature)
}

func TestGetDestination_NotFound(t *testing.T) {
	q := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &fakeRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	dest, err := repo.GetDestination(context.Background(), "Atlantis")
	require.NoError(t, err)
	assert.Nil(t, dest)
}

func TestGetDestination_DBError(t *testing.T) {
	q := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &fakeRow{scanFn: func(dest ...any) error { return fmt.Errorf("connection reset") }}
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestination(context.Background(), "Paris")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "querying destination")
}

func TestGetDestination_BadJSON(t *testing.T) {
	now := time.Now()
	q := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &fakeRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				*dest[1].(*string) = "Paris"
				*dest[2].(*string) = "France"
				*dest[3].(*[]byte) = []byte("not-valid-json")
				*dest[4].(**time.Time) = &now
				*dest[5].(*time.Time) = now
				*dest[6].(*time.Time) = now
				return nil
			}}
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestination(context.Background(), "Paris")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling")
}

// ---- UpsertDestination tests ----

func TestUpsertDestination_Success(t *testing.T) {
	var capturedArgs []any
	q := &mockQuerier{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = args
			return pgconn.CommandTag{}, nil
		},
	}

	data := destination.DestinationData{
		Weather: &destination.WeatherData{Temperature: 20.0},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	err := repo.UpsertDestination(context.Background(), "Paris", "France", data)
	require.NoError(t, err)
	require.Len(t, capturedArgs, 3)
	assert.Equal(t, "Paris", capturedArgs[0])
	assert.Equal(t, "France", capturedArgs[1])
}

func TestUpsertDestination_DBError(t *testing.T) {
	q := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fmt.Errorf("db error")
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	err := repo.UpsertDestination(context.Background(), "Paris", "France", destination.DestinationData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upserting destination")
}

// ---- GetDestinationByWeatherCondition tests ----

func TestGetDestinationByWeatherCondition_Found(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	data := destination.DestinationData{
		Weather: &destination.WeatherData{Temperature: 15.0, Description: "clear sky"},
	}
	dataJSON := marshalData(t, data)

	rows := &fakeRows{
		rows: [][]any{{1, "Paris", "France", dataJSON, nil, now, now}},
	}

	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	results, err := repo.GetDestinationByWeatherCondition(context.Background(), "clear sky")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Paris", results[0].City)
}

func TestGetDestinationByWeatherCondition_Empty(t *testing.T) {
	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &fakeRows{}, nil
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	results, err := repo.GetDestinationByWeatherCondition(context.Background(), "blizzard")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestGetDestinationByWeatherCondition_QueryError(t *testing.T) {
	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, fmt.Errorf("query failed")
		},
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestinationByWeatherCondition(context.Background(), "rain")
	require.Error(t, err)
}

func TestGetDestinationByWeatherCondition_ScanError(t *testing.T) {
	now := time.Now()
	rows := &fakeRows{
		rows:    [][]any{{1, "Paris", "France", []byte("{}"), &now, now, now}},
		scanErr: fmt.Errorf("scan failed"),
	}

	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return rows, nil },
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestinationByWeatherCondition(context.Background(), "clear sky")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scanning")
}

func TestGetDestinationByWeatherCondition_RowsErr(t *testing.T) {
	rows := &fakeRows{rowErr: fmt.Errorf("rows iteration error")}

	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return rows, nil },
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestinationByWeatherCondition(context.Background(), "clear sky")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iterating")
}

func TestGetDestinationByWeatherCondition_BadJSON(t *testing.T) {
	now := time.Now()
	rows := &fakeRows{
		rows: [][]any{{1, "Paris", "France", []byte("not-json"), nil, now, now}},
	}

	q := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return rows, nil },
	}

	repo := storage.NewRepositoryWithQuerier(q)
	_, err := repo.GetDestinationByWeatherCondition(context.Background(), "clear sky")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling")
}

// ---- NewRepository ----

func TestNewRepository_NotNil(t *testing.T) {
	repo := storage.NewRepository(nil)
	assert.NotNil(t, repo)
}

// ---- RunMigrations tests ----

func TestRunMigrations_MissingDir(t *testing.T) {
	err := storage.RunMigrations(context.Background(), nil, "/nonexistent/dir")
	require.Error(t, err)
}

func TestRunMigrations_EmptyDir(t *testing.T) {
	err := storage.RunMigrations(context.Background(), nil, t.TempDir())
	require.NoError(t, err)
}

func TestRunMigrations_Success(t *testing.T) {
	dir := t.TempDir()
	writeSQLFile(t, dir, "001_test.sql", "SELECT 1;")

	tx := &mockTx{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		commitFn:   func(_ context.Context) error { return nil },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	pool := &mockMigrationPool{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}

	err := storage.RunMigrations(context.Background(), pool, dir)
	require.NoError(t, err)
}

func TestRunMigrations_BeginError(t *testing.T) {
	dir := t.TempDir()
	writeSQLFile(t, dir, "001_test.sql", "SELECT 1;")

	pool := &mockMigrationPool{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return nil, fmt.Errorf("cannot begin") },
	}

	err := storage.RunMigrations(context.Background(), pool, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executing migration")
}

func TestRunMigrations_ExecError(t *testing.T) {
	dir := t.TempDir()
	writeSQLFile(t, dir, "001_test.sql", "INVALID SQL;")

	tx := &mockTx{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fmt.Errorf("syntax error")
		},
		commitFn:   func(_ context.Context) error { return nil },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	pool := &mockMigrationPool{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}

	err := storage.RunMigrations(context.Background(), pool, dir)
	require.Error(t, err)
}

func TestRunMigrations_CommitError(t *testing.T) {
	dir := t.TempDir()
	writeSQLFile(t, dir, "001_test.sql", "SELECT 1;")

	tx := &mockTx{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		commitFn:   func(_ context.Context) error { return fmt.Errorf("commit failed") },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	pool := &mockMigrationPool{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}

	err := storage.RunMigrations(context.Background(), pool, dir)
	require.Error(t, err)
}

func TestRunMigrations_SortsFilesLexicographically(t *testing.T) {
	dir := t.TempDir()
	var order []string
	writeSQLFile(t, dir, "003_c.sql", "SELECT 3;")
	writeSQLFile(t, dir, "001_a.sql", "SELECT 1;")
	writeSQLFile(t, dir, "002_b.sql", "SELECT 2;")

	tx := &mockTx{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			order = append(order, sql)
			return pgconn.CommandTag{}, nil
		},
		commitFn:   func(_ context.Context) error { return nil },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	pool := &mockMigrationPool{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}

	err := storage.RunMigrations(context.Background(), pool, dir)
	require.NoError(t, err)
	require.Len(t, order, 3)
	assert.Equal(t, "SELECT 1;", order[0])
	assert.Equal(t, "SELECT 2;", order[1])
	assert.Equal(t, "SELECT 3;", order[2])
}

// ---- Connect tests ----

func TestConnect_BadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := storage.Connect(ctx, "postgres://invalid-host-xyz:5432/db?sslmode=disable")
	require.Error(t, err)
}
