package ai.loomspan.cloud.engine;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.ErrorResponse;
import org.springframework.web.servlet.HandlerExceptionResolver;
import org.springframework.web.servlet.ModelAndView;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.json.JsonMapper;

import java.io.IOException;
import java.io.UncheckedIOException;

@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
final class InvocationExceptionResolver implements HandlerExceptionResolver {
    private final JsonMapper mapper;

    InvocationExceptionResolver(JsonMapper mapper) {
        this.mapper = mapper;
    }

    @Override
    public ModelAndView resolveException(HttpServletRequest request, HttpServletResponse response, Object handler,
                                         Exception failure) {
        String path = request.getRequestURI().substring(request.getContextPath().length());
        if (!"POST".equals(request.getMethod()) || !InvocationController.PATH.equals(path) || response.isCommitted()) {
            return null;
        }
        int status = HttpStatus.INTERNAL_SERVER_ERROR.value();
        if (failure instanceof ErrorResponse webFailure && webFailure.getStatusCode().is4xxClientError()) {
            status = webFailure.getStatusCode().value();
        } else if (failure instanceof IllegalArgumentException || failure instanceof JacksonException) {
            status = HttpStatus.BAD_REQUEST.value();
        }
        response.setStatus(status);
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        try {
            mapper.writeValue(response.getOutputStream(), InvocationController.failureBody(failure));
        } catch (IOException disconnected) {
            throw new UncheckedIOException(disconnected);
        }
        return new ModelAndView();
    }
}
