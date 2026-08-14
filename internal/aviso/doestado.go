package aviso

// Ponte entre a configuração do projeto e a central.
//
// Fica em arquivo próprio de propósito: o núcleo do pacote (aviso.go) não conhece
// `config`, e é isso que o mantém genérico — dá para notificar de qualquer lugar sem
// arrastar a configuração do gokit. Só esta ponte sabe onde ficam o webhook e o filtro.
//
// Existe porque a alternativa era cada chamador montar a central do zero, e montagem
// duplicada sai de sincronia: um lugar passaria o dialeto, outro esqueceria, e o mesmo
// erro chegaria ao canal com informação diferente conforme o caminho.

import "github.com/PhelipeViana/gokit/internal/config"

// versao é a do binário, injetada uma vez pelo main. Fica em variável de pacote para
// não ter de ser passada em cada chamada de notificação, que são muitas e espalhadas.
var versao = "development"

// DefinirVersao registra a versão do binário para os relatórios.
func DefinirVersao(v string) {
	if v != "" {
		versao = v
	}
}

// DoEstado monta a central a partir da configuração do projeto.
//
// Sem webhook ou sem `enabled`, ela nasce sem canal e tudo é no-op: notificação é
// opt-in por projeto, e ninguém recebe nada por acidente.
func DoEstado(state config.ConfigState) Central {
	if state.Config == nil {
		return Central{}
	}
	return Nova(
		Ambiente{
			Projeto: state.Config.Go.Module,
			Cliente: state.ActiveClient,
			Dialeto: state.ActiveDialect,
			Versao:  versao,
		},
		state.Config.Notifications.Niveis,
		NovoSlack(state.Config.Notifications.Slack.WebhookURL, state.Config.Notifications.Slack.Enabled),
	)
}
