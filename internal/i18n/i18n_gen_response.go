package i18n

// Comentários emitidos no response.gen.go (padrão de resposta HTTP gerado).
func init() {
	Register(Entradas{
		"gen_resp_pluggable": {
			PT: "Ajustes plugáveis (a aplicação seta no boot, se quiser)",
			ES: "Ajustes conectables (la aplicación los define en el arranque, si quiere)",
			EN: "Pluggable settings (the application may set these at boot)",
		},
		"gen_resp_requestid": {
			PT: "RequestIDFn extrai o request_id do contexto (ex.: um middleware). Padrão: vazio.",
			ES: "RequestIDFn extrae el request_id del contexto (ej.: un middleware). Por defecto: vacío.",
			EN: "RequestIDFn pulls the request_id from the context (e.g. a middleware). Default: empty.",
		},
		"gen_resp_local": {
			PT: "Local é o fuso usado no timestamp de meta. Padrão: fuso local do servidor.",
			ES: "Local es la zona usada en el timestamp de meta. Por defecto: zona local del servidor.",
			EN: "Local is the time zone used in meta's timestamp. Default: the server's local zone.",
		},
		"gen_resp_statustext": {
			PT: "DefaultStatusText devolve a mensagem padrão para um status HTTP.",
			ES: "DefaultStatusText devuelve el mensaje estándar para un status HTTP.",
			EN: "DefaultStatusText returns the default message for an HTTP status.",
		},
		"gen_resp_meta": {
			PT: "Meta são os metadados que acompanham toda resposta.",
			ES: "Meta son los metadatos que acompañan toda respuesta.",
			EN: "Meta holds the metadata attached to every response.",
		},
		"gen_resp_success": {
			PT: "Success responde com sucesso (payload padrão) no status informado.",
			ES: "Success responde con éxito (payload estándar) en el status informado.",
			EN: "Success replies successfully (standard payload) with the given status.",
		},
		"gen_resp_success_paginated": {
			PT: "SuccessPaginated responde com sucesso incluindo os metadados de paginação em meta.",
			ES: "SuccessPaginated responde con éxito incluyendo los metadatos de paginación en meta.",
			EN: "SuccessPaginated replies successfully including pagination metadata under meta.",
		},
		"gen_resp_error": {
			PT: "Error responde um erro no status informado, aceitando string ou error como mensagem.",
			ES: "Error responde un error en el status informado, aceptando string o error como mensaje.",
			EN: "Error replies with an error at the given status, accepting a string or an error as message.",
		},
		"gen_resp_error_details": {
			PT: "ErrorWithDetails responde um erro com mensagem e detalhes por campo.",
			ES: "ErrorWithDetails responde un error con mensaje y detalles por campo.",
			EN: "ErrorWithDetails replies with an error message plus per-field details.",
		},
		"gen_resp_emit": {
			PT: "emit entrega o payload à Central de Resposta quando ela está no contexto; sem Central, serializa direto.",
			ES: "emit entrega el payload a la Central de Respuesta cuando está en el contexto; sin Central, serializa directo.",
			EN: "emit hands the payload to the Response Center when present in the context; without it, serializes directly.",
		},
		"gen_resp_emit_error": {
			PT: "emitError aplica o mascaramento de 5xx (safeError) só quando NÃO há Central.",
			ES: "emitError aplica el enmascarado de 5xx (safeError) solo cuando NO hay Central.",
			EN: "emitError applies 5xx masking (safeError) only when there is no Response Center.",
		},
		"gen_resp_safeerror": {
			PT: "safeError mascara falhas 5xx: registra a causa real com o request_id e devolve mensagem genérica.",
			ES: "safeError enmascara fallos 5xx: registra la causa real con el request_id y devuelve mensaje genérico.",
			EN: "safeError masks 5xx failures: logs the real cause with the request_id and returns a generic message.",
		},
		"gen_resp_writejson": {
			PT: "WriteJSON serializa o payload como JSON e escreve a resposta com o status informado.",
			ES: "WriteJSON serializa el payload como JSON y escribe la respuesta con el status informado.",
			EN: "WriteJSON serializes the payload as JSON and writes the response with the given status.",
		},
		"gen_resp_recorder": {
			PT: "Central de Resposta (Recorder): hook opcional que a plataforma HTTP implementa para pós-processar a resposta.",
			ES: "Central de Respuesta (Recorder): hook opcional que la plataforma HTTP implementa para posprocesar la respuesta.",
			EN: "Response Center (Recorder): optional hook the HTTP platform implements to post-process the response.",
		},
		"gen_resp_pagination_parse": {
			PT: "ParsePagination lê page/per_page da query e valida, com erros por campo.",
			ES: "ParsePagination lee page/per_page de la query y valida, con errores por campo.",
			EN: "ParsePagination reads page/per_page from the query and validates them, with per-field errors.",
		},
		"gen_resp_pagination_info": {
			PT: "NewPaginationInfo monta os metadados de paginação a partir do total e dos parâmetros.",
			ES: "NewPaginationInfo arma los metadatos de paginación a partir del total y los parámetros.",
			EN: "NewPaginationInfo builds the pagination metadata from the total and the parameters.",
		},
		"gen_resp_bridge": {
			PT: "Ponte com a ORM: o handler só monta a query e chama um Respond*",
			ES: "Puente con el ORM: el handler solo arma la query y llama a un Respond*",
			EN: "Bridge to the ORM: the handler only builds the query and calls a Respond*",
		},
		"gen_resp_statuserror": {
			PT: "StatusError carrega um status HTTP + mensagem; o responder deriva o código dele.",
			ES: "StatusError lleva un status HTTP + mensaje; el responder deriva el código de él.",
			EN: "StatusError carries an HTTP status + message; the responder derives the code from it.",
		},
		"gen_resp_list": {
			PT: "RespondList valida page/per_page, pagina a query e responde paginado.",
			ES: "RespondList valida page/per_page, pagina la query y responde paginado.",
			EN: "RespondList validates page/per_page, paginates the query and replies paginated.",
		},
		"gen_resp_item": {
			PT: "RespondItem responde {data}; o 404 sai do próprio erro (ErrNotFound) e a mensagem de 5xx é mascarada.",
			ES: "RespondItem responde {data}; el 404 sale del propio error (ErrNotFound) y el mensaje de 5xx se enmascara.",
			EN: "RespondItem replies {data}; the 404 comes from the error itself (ErrNotFound) and 5xx messages are masked.",
		},
		"gen_resp_data": {
			PT: "RespondData responde {data} para um payload arbitrário (estatísticas, agregações...).",
			ES: "RespondData responde {data} para un payload arbitrario (estadísticas, agregaciones...).",
			EN: "RespondData replies {data} for an arbitrary payload (stats, aggregations...).",
		},
		"gen_resp_err": {
			PT: "RespondError responde um erro com o status derivado dele.",
			ES: "RespondError responde un error con el status derivado de él.",
			EN: "RespondError replies with an error using the status derived from it.",
		},
		"gen_resp_envelope_success": {
			PT: "SuccessPayload é o envelope de sucesso da API.",
			ES: "SuccessPayload es el envelope de éxito de la API.",
			EN: "SuccessPayload is the API's success envelope.",
		},
		"gen_resp_envelope_error": {
			PT: "ErrorPayload é o envelope de erro da API.",
			ES: "ErrorPayload es el envelope de error de la API.",
			EN: "ErrorPayload is the API's error envelope.",
		},
	})
}
