package aviso

// Canal de Slack: um webhook de entrada.
//
// É opt-in por projeto, e de propósito. O scaffold nasce com `enabled: false` e a URL
// vem de `${SLACK_WEBHOOK_URL}`: sem a variável no .env e sem ligar, o canal é inerte
// e ninguém recebe nada por acidente. Quem quiser acompanhar os próprios erros
// configura o seu.
//
// A URL nunca aparece em código nem em arquivo versionado. Webhook é credencial —
// quem tem a URL posta no canal — e é por isso que o .env entrou no .gitignore.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Slack struct {
	webhook string
}

// NovoSlack devolve o canal, ou nil quando não há para onde enviar. Nil é tratado
// pela Central, então quem chama não precisa checar.
func NovoSlack(webhook string, ativo bool) Canal {
	url := strings.TrimSpace(webhook)
	if !ativo || !strings.HasPrefix(url, "https://") {
		return nil
	}
	return Slack{webhook: url}
}

func (s Slack) Nome() string { return "Slack" }

func (s Slack) Enviar(texto string) error {
	dados, err := json.Marshal(map[string]string{"text": texto})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	requisicao, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhook, bytes.NewReader(dados))
	if err != nil {
		return err
	}
	requisicao.Header.Set("Content-Type", "application/json")

	resposta, err := http.DefaultClient.Do(requisicao)
	if err != nil {
		return err
	}
	defer resposta.Body.Close()
	if resposta.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resposta.StatusCode)
	}
	return nil
}
