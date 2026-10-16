export function validate(schema) {
	return (request, response, next) => {
		request.validated = schema.parse({ body: request.body, query: request.query, params: request.params });
		next();
	};
}
