package i18n

// Mensagens dos pacotes de apoio: atualização do binário, leitura de AST,
// configuração (gokit.json), go.mod e a formatação de erro com solução.
func init() {
	Register(Entradas{
		// ── Atualização do binário ──
		"upd_remote_version_failed": {
			PT: "não foi possível obter a versão remota (status %d)",
			ES: "no se pudo obtener la versión remota (status %d)",
			EN: "could not fetch the remote version (status %d)",
		},
		"upd_exec_unknown": {
			PT: "não foi possível identificar o executável: %v",
			ES: "no se pudo identificar el ejecutable: %v",
			EN: "could not identify the executable: %v",
		},
		"upd_network_failed": {
			PT: "falha na conexão de rede: %v",
			ES: "fallo en la conexión de red: %v",
			EN: "network connection failed: %v",
		},
		"upd_server_error": {
			PT: "servidor retornou erro (status %d)",
			ES: "el servidor retornó error (status %d)",
			EN: "the server returned an error (status %d)",
		},
		"upd_prepare_failed": {
			PT: "falha ao preparar download: %v",
			ES: "fallo al preparar la descarga: %v",
			EN: "failed to prepare the download: %v",
		},
		"upd_write_failed": {
			PT: "falha ao gravar atualização: %v",
			ES: "fallo al grabar la actualización: %v",
			EN: "failed to write the update: %v",
		},
		"upd_finish_failed": {
			PT: "falha ao finalizar atualização: %v",
			ES: "fallo al finalizar la actualización: %v",
			EN: "failed to finalize the update: %v",
		},
		"upd_download_too_small": {
			PT: "download inválido: executável recebido tem apenas %d bytes",
			ES: "descarga inválida: el ejecutable recibido tiene solo %d bytes",
			EN: "invalid download: the received executable is only %d bytes",
		},
		"upd_chmod_failed": {
			PT: "falha ao tornar atualização executável: %v",
			ES: "fallo al hacer ejecutable la actualización: %v",
			EN: "failed to make the update executable: %v",
		},
		"upd_backup_failed": {
			PT: "falha ao preservar executável atual: %v",
			ES: "fallo al preservar el ejecutable actual: %v",
			EN: "failed to preserve the current executable: %v",
		},
		"upd_install_failed": {
			PT: "falha ao instalar atualização: %v",
			ES: "fallo al instalar la actualización: %v",
			EN: "failed to install the update: %v",
		},
		"upd_macos_validate_failed": {
			PT: "falha ao validar atualização no macOS: %v",
			ES: "fallo al validar la actualización en macOS: %v",
			EN: "failed to validate the update on macOS: %v",
		},
		// O pior desfecho da atualização: falhou E o binário antigo não voltou. A
		// mensagem tem de trazer os dois caminhos, porque a saída é renomear à mão — e
		// quem lê isso não tem mais o comando para pedir ajuda a ele.
		"upd_restore_failed": {
			PT: "%w — e o executável anterior NÃO voltou ao lugar: renomeie %s para %s à mão para ter o gokit de volta (%v)",
			ES: "%w — y el ejecutable anterior NO volvió a su lugar: renombre %s a %s a mano para recuperar gokit (%v)",
			EN: "%w — and the previous executable was NOT restored: rename %s to %s by hand to get gokit back (%v)",
		},
		"upd_codesign": {
			PT: "codesign: %s: %w",
			ES: "codesign: %s: %w",
			EN: "codesign: %s: %w",
		},

		// ── Leitura de AST ──
		"ast_expect_quoted": {
			PT: "esperado texto entre aspas",
			ES: "se esperaba un texto entre comillas",
			EN: "expected a quoted string",
		},
		"ast_expect_int": {
			PT: "esperado número inteiro",
			ES: "se esperaba un número entero",
			EN: "expected an integer",
		},
		"ast_not_call_chain": {
			PT: "expressão não reconhecida como cadeia de chamadas",
			ES: "expresión no reconocida como cadena de llamadas",
			EN: "expression not recognized as a call chain",
		},

		// ── Configuração ──
		"cfg_gomod_init_failed": {
			PT: "nao foi possivel inicializar o modulo Go: %w",
			ES: "no se pudo inicializar el modulo Go: %w",
			EN: "could not initialize the Go module: %w",
		},
		"cfg_read_failed": {
			PT: "não foi possível ler o arquivo: %v",
			ES: "no se pudo leer el archivo: %v",
			EN: "could not read the file: %v",
		},
		"cfg_bad_json": {
			PT: "sintaxe JSON inválida: %v",
			ES: "sintaxis JSON inválida: %v",
			EN: "invalid JSON syntax: %v",
		},
		"cfg_client_not_found": {
			PT: "cliente ativo '%s' não encontrado. Suas conexões configuradas no gokit.json são: %s",
			ES: "cliente activo '%s' no encontrado. Sus conexiones configuradas en el gokit.json son: %s",
			EN: "active client '%s' not found. The connections configured in gokit.json are: %s",
		},
		"mod_invalid": {
			PT: "go.mod invalido: %w",
			ES: "go.mod invalido: %w",
			EN: "invalid go.mod: %w",
		},
		"mod_dev_sem_local": {
			PT: "o modo dev exige o gokit ao lado do projeto, e não há um checkout em %s. Clone o gokit nesse caminho ou rode: gokit mode prod",
			ES: "el modo dev exige gokit al lado del proyecto, y no hay un checkout en %s. Clone gokit en esa ruta o ejecute: gokit mode prod",
			EN: "dev mode requires gokit next to the project, and there is no checkout at %s. Clone gokit there, or run: gokit mode prod",
		},
		"mod_gowork_falhou": {
			PT: "ajustar o go.work: %w",
			ES: "ajustar el go.work: %w",
			EN: "adjusting go.work: %w",
		},
		"mod_local_not_found": {
			PT: "codigo local do GoKit nao encontrado em %s",
			ES: "codigo local de GoKit no encontrado en %s",
			EN: "GoKit local source not found at %s",
		},

		// ── Modo do projeto (dev | prod) ──
		"modo_desconhecido": {
			PT: "modo %q não existe; use um destes: %s",
			ES: "el modo %q no existe; use uno de estos: %s",
			EN: "mode %q does not exist; use one of: %s",
		},
		"cli_mode_current": {
			PT: "Modo atual: %s",
			ES: "Modo actual: %s",
			EN: "Current mode: %s",
		},
		"cli_mode_usage": {
			PT: "Para trocar: gokit mode dev  ·  gokit mode prod\n\n  dev   compila contra o gokit da pasta irmã, por go.work (não commitado)\n  prod  fixa a versão publicada do gokit no go.mod, sem depender de pasta local",
			ES: "Para cambiar: gokit mode dev  ·  gokit mode prod\n\n  dev   compila contra el gokit de la carpeta hermana, por go.work (no confirmado)\n  prod  fija la versión publicada de gokit en el go.mod, sin depender de carpeta local",
			EN: "To switch: gokit mode dev  ·  gokit mode prod\n\n  dev   builds against the gokit in the sibling folder, through go.work (not committed)\n  prod  pins the published gokit version in go.mod, with no local folder involved",
		},
		"cli_mode_conflito": {
			PT: "  ⚠️ %s redireciona o gokit para uma pasta local e vale para este projeto.\n     O modo diz prod, mas o build usa código não publicado. Remova a entrada do gokit desse go.work, ou rode com GOWORK=off para conferir.",
			ES: "  ⚠️ %s redirige gokit a una carpeta local y aplica a este proyecto.\n     El modo dice prod, pero el build usa código no publicado. Quite la entrada de gokit de ese go.work, o ejecute con GOWORK=off para comprobar.",
			EN: "  ⚠️ %s redirects gokit to a local folder and applies to this project.\n     The mode says prod, but the build uses unpublished code. Remove the gokit entry from that go.work, or run with GOWORK=off to check.",
		},
		"cli_mode_changed": {
			PT: "Modo alterado para: %s",
			ES: "Modo cambiado a: %s",
			EN: "Mode changed to: %s",
		},
		"cli_mode_dev_detail": {
			PT: "  go.work aponta para %s — cada alteração no gokit vale na hora.",
			ES: "  go.work apunta a %s — cada cambio en gokit vale de inmediato.",
			EN: "  go.work points at %s — every change in gokit takes effect immediately.",
		},
		"cli_mode_prod_detail": {
			PT: "  go.mod fixa %s %s — nenhuma pasta local participa do build.",
			ES: "  go.mod fija %s %s — ninguna carpeta local participa del build.",
			EN: "  go.mod pins %s %s — no local folder takes part in the build.",
		},

		// ── Erro com solução (cliui) ──
		"cli_error_with_fix": {
			PT: "%s (Solução: %s)",
			ES: "%s (Solución: %s)",
			EN: "%s (Fix: %s)",
		},
	})
}
