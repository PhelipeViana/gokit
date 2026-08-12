package i18n

// Mensagens que vão DENTRO do response.gen.go gerado: os textos padrão por
// status HTTP e as validações de paginação. Diferente dos comentários, estas
// frases chegam ao usuário final da API do cliente.
//
// Saem no idioma vigente no momento da GERAÇÃO — trocar o idioma do projeto
// exige regerar o response.gen.go.
func init() {
	Register(Entradas{
		// ── Sucesso ──
		"gen_msg_ok": {
			PT: "Operação realizada com sucesso",
			ES: "Operación realizada con éxito",
			EN: "Operation completed successfully",
		},
		"gen_msg_created": {
			PT: "Criado com sucesso",
			ES: "Creado con éxito",
			EN: "Created successfully",
		},
		"gen_msg_accepted": {
			PT: "Aceito",
			ES: "Aceptado",
			EN: "Accepted",
		},
		"gen_msg_no_content": {
			PT: "Sem conteúdo",
			ES: "Sin contenido",
			EN: "No content",
		},

		// ── Erro do cliente ──
		"gen_msg_bad_request": {
			PT: "Requisição inválida",
			ES: "Solicitud inválida",
			EN: "Bad request",
		},
		"gen_msg_unauthorized": {
			PT: "Não autorizado: sessão inválida ou ausente",
			ES: "No autorizado: sesión inválida o ausente",
			EN: "Unauthorized: session invalid or missing",
		},
		"gen_msg_forbidden": {
			PT: "Acesso proibido",
			ES: "Acceso prohibido",
			EN: "Forbidden",
		},
		"gen_msg_not_found": {
			PT: "Recurso não encontrado",
			ES: "Recurso no encontrado",
			EN: "Resource not found",
		},
		"gen_msg_method_not_allowed": {
			PT: "Método não permitido",
			ES: "Método no permitido",
			EN: "Method not allowed",
		},
		"gen_msg_conflict": {
			PT: "Conflito",
			ES: "Conflicto",
			EN: "Conflict",
		},
		"gen_msg_unprocessable": {
			PT: "Entidade não processável",
			ES: "Entidad no procesable",
			EN: "Unprocessable entity",
		},
		"gen_msg_too_many": {
			PT: "Muitas requisições",
			ES: "Demasiadas solicitudes",
			EN: "Too many requests",
		},

		// ── Erro do servidor ──
		"gen_msg_internal": {
			PT: "Erro interno do servidor.",
			ES: "Error interno del servidor.",
			EN: "Internal server error.",
		},
		"gen_msg_not_implemented": {
			PT: "Não implementado",
			ES: "No implementado",
			EN: "Not implemented",
		},
		"gen_msg_bad_gateway": {
			PT: "Gateway inválido",
			ES: "Gateway inválido",
			EN: "Bad gateway",
		},
		"gen_msg_unavailable": {
			PT: "Serviço indisponível",
			ES: "Servicio no disponible",
			EN: "Service unavailable",
		},
		"gen_msg_gateway_timeout": {
			PT: "Tempo limite do gateway esgotado",
			ES: "Tiempo límite del gateway agotado",
			EN: "Gateway timeout",
		},
		"gen_msg_internal_log": {
			PT: "erro interno request_id=%s: %s",
			ES: "error interno request_id=%s: %s",
			EN: "internal error request_id=%s: %s",
		},

		// ── Validação de paginação ──
		"gen_msg_must_be_int": {
			PT: "Campo deve ser inteiro válido",
			ES: "El campo debe ser un entero válido",
			EN: "Field must be a valid integer",
		},
		"gen_msg_must_be_positive": {
			PT: "Valor deve ser maior que zero",
			ES: "El valor debe ser mayor que cero",
			EN: "Value must be greater than zero",
		},
		"gen_msg_max_per_page": {
			PT: "Valor deve ser menor ou igual a 100",
			ES: "El valor debe ser menor o igual a 100",
			EN: "Value must be less than or equal to 100",
		},
		// Erro do loader de relação gerado no core.gen.go: só acontece se o
		// motor passar uma fatia de outro tipo, então é diagnóstico interno.
		"gen_msg_wrong_parents": {
			PT: "relação %s: pais não são []%s",
			ES: "relación %s: los padres no son []%s",
			EN: "relation %s: parents are not []%s",
		},
		"gen_msg_bad_pagination": {
			PT: "Parâmetros de paginação inválidos",
			ES: "Parámetros de paginación inválidos",
			EN: "Invalid pagination parameters",
		},
	})
}
