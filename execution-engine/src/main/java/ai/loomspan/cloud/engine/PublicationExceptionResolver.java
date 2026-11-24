package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillReloadException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.ErrorResponse;
import org.springframework.web.multipart.MultipartException;
import org.springframework.web.servlet.HandlerExceptionResolver;
import org.springframework.web.servlet.ModelAndView;
import tools.jackson.core.JacksonException;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;

/** Includes multipart failures raised by the servlet resolver before handler selection. */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
final class PublicationExceptionResolver implements HandlerExceptionResolver {
    @Override
    public ModelAndView resolveException(HttpServletRequest request, HttpServletResponse response, Object handler,
                                         Exception failure) {
        String path = request.getRequestURI().substring(request.getContextPath().length());
        if (!"POST".equals(request.getMethod()) || response.isCommitted()
                || !(PublicationController.VALIDATE_PATH.equals(path) || PublicationController.PUBLISH_PATH.equals(path))) {
            return null;
        }
        HttpStatus status = HttpStatus.INTERNAL_SERVER_ERROR;
        if (failure instanceof ErrorResponse webFailure) {
            response.setStatus(webFailure.getStatusCode().value());
        } else {
            if (failure instanceof IllegalArgumentException || failure instanceof JacksonException
                    || failure instanceof MultipartException || failure instanceof SkillReloadException) {
                status = HttpStatus.BAD_REQUEST;
            }
            response.setStatus(status.value());
        }
        response.setContentType(MediaType.TEXT_PLAIN_VALUE);
        response.setCharacterEncoding(StandardCharsets.UTF_8);
        try {
            String message = failure.getMessage();
            response.getWriter().write(message == null ? failure.getClass().getSimpleName() : message);
        } catch (IOException disconnected) {
            throw new UncheckedIOException(disconnected);
        }
        return new ModelAndView();
    }
}
