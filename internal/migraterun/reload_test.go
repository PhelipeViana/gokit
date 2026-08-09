package migraterun

import "testing"

func TestContainsComposeService(t *testing.T) {
	services := "toolchain\nmysql\npostgres\n"
	if !containsComposeService(services, "mysql") {
		t.Fatal("serviço mysql deveria ser encontrado")
	}
	if containsComposeService(services, "sql") {
		t.Fatal("a busca deve comparar o nome completo do serviço")
	}
}

func TestReloadStartsDatabaseBeforePing(t *testing.T) {
	pipeline := InitReloadPipeline()
	startIndex, pingIndex := -1, -1
	for index, step := range pipeline.Group1 {
		switch step.Name {
		case "start_database":
			startIndex = index
		case "check_conn":
			pingIndex = index
		}
	}
	if startIndex < 0 || pingIndex < 0 || startIndex >= pingIndex {
		t.Fatalf("ordem inválida: start_database=%d check_conn=%d", startIndex, pingIndex)
	}
}
