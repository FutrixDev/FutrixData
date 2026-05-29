package console

func ParseMongoTarget(statement string) (string, string, error) {
	stmt, err := parseMongoStatement(statement)
	if err != nil {
		return "", "", err
	}
	return stmt.Collection, stmt.Action, nil
}
