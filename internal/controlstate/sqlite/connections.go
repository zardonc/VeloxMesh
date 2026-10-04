package sqlite

import (
	"fmt"
	"net/url"
	"strings"
)

const sqliteBusyTimeoutMS = 5000

// PRAGMAs executed through sql.DB configure only the selected connection.
// Driver DSN pragmas are applied whenever the pool opens another connection.
func connectionDSN(dsn string) (string, error) {
	base, query, _ := strings.Cut(dsn, "?")
	values, err := url.ParseQuery(query)
	if err != nil {
		return "", fmt.Errorf("invalid SQLite connection parameters: %w", err)
	}
	values.Add("_pragma", "foreign_keys(1)")
	values.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMS))
	values.Add("_pragma", "synchronous(NORMAL)")
	return base + "?" + values.Encode(), nil
}
