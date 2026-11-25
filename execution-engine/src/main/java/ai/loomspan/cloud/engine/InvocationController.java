package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillException;
import ai.loomspan.api.SkillInputValidationException;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;
import tools.jackson.core.StreamReadFeature;
import tools.jackson.core.type.TypeReference;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectReader;
import tools.jackson.databind.json.JsonMapper;

import java.io.IOException;
import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Map;

@RestController
final class InvocationController {
    static final String PATH = "/internal/v1/invoke";
    private static final TypeReference<Map<String, Object>> INPUT = new TypeReference<>() {};

    private final InvocationService service;
    private final JsonMapper mapper;
    private final ObjectReader reader;

    InvocationController(InvocationService service, JsonMapper mapper) {
        this.service = service;
        this.mapper = mapper;
        this.reader = mapper.reader().with(StreamReadFeature.STRICT_DUPLICATE_DETECTION)
                .with(DeserializationFeature.USE_BIG_DECIMAL_FOR_FLOATS)
                .with(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);
    }

    @PostMapping(path = PATH, consumes = MediaType.APPLICATION_JSON_VALUE, produces = MediaType.APPLICATION_JSON_VALUE)
    ResponseEntity<Map<String, Object>> invoke(HttpServletRequest request) throws IOException {
        JsonNode root;
        try (var stream = request.getInputStream()) {
            root = reader.readTree(stream);
        }
        requireObject(root, "request");
        JsonNode mode = root.get("executionView");
        boolean includeView = mode != null;
        if (includeView && !(mode.isString() && "always".equals(mode.stringValue()))) {
            throw new IllegalArgumentException("executionView must be always when supplied");
        }
        try {
            var caller = requireObject(root.get("caller"), "caller");
            String issuer = requireText(caller.get("issuer"), "caller.issuer");
            String subject = requireText(caller.get("subject"), "caller.subject");
            var roleNodes = caller.get("roles");
            if (roleNodes == null || !roleNodes.isArray()) {
                throw new IllegalArgumentException("caller.roles must be an array");
            }
            var roles = new ArrayList<String>(roleNodes.size());
            int index = 0;
            for (JsonNode role : roleNodes) {
                roles.add(requireText(role, "caller.roles[" + index++ + "]"));
            }
            String namespace = requireText(root.get("namespace"), "namespace");
            String skill = requireText(root.get("skill"), "skill");
            var input = mapper.convertValue(requireObject(root.get("input"), "input"), INPUT);
            var result = service.invoke(namespace, skill, input, new InvocationService.Caller(issuer, subject, roles),
                    includeView);
            var details = new LinkedHashMap<String, Object>();
            HttpStatus status;
            if (result.failure() == null) {
                details.put("result", result.result());
                status = HttpStatus.OK;
            } else {
                details.putAll(failureBody(result.failure()));
                status = invocationFailureStatus(result.failure());
            }
            if (includeView) {
                details.put("executionView", result.view());
            }
            return ResponseEntity.status(status).contentType(MediaType.APPLICATION_JSON).body(details);
        } catch (IllegalArgumentException invalid) {
            var details = failureBody(invalid);
            if (includeView) {
                details.put("executionView", null);
            }
            return ResponseEntity.badRequest().contentType(MediaType.APPLICATION_JSON).body(details);
        }
    }

    private static JsonNode requireObject(JsonNode value, String path) {
        if (value == null || !value.isObject()) {
            throw new IllegalArgumentException(path + " must be an object");
        }
        return value;
    }

    private static String requireText(JsonNode value, String path) {
        if (value == null || !value.isString() || value.stringValue().isBlank()) {
            throw new IllegalArgumentException(path + " must be a nonblank string");
        }
        return value.stringValue();
    }

    private static HttpStatus invocationFailureStatus(RuntimeException failure) {
        if (InvocationAdmissionRejection.isConfirmed(failure)) {
            return HttpStatus.SERVICE_UNAVAILABLE;
        }
        if (failure instanceof AccessDeniedException) {
            return HttpStatus.FORBIDDEN;
        }
        if (failure instanceof SkillInputValidationException || failure instanceof IllegalArgumentException) {
            return HttpStatus.BAD_REQUEST;
        }
        return HttpStatus.INTERNAL_SERVER_ERROR;
    }

    static Map<String, Object> failureBody(Exception failure) {
        var stack = new StringWriter();
        failure.printStackTrace(new PrintWriter(stack));
        var details = new LinkedHashMap<String, Object>();
        details.put("message", failure.getMessage());
        details.put("debug", stack.toString());
        if (failure instanceof SkillException skillFailure) {
            skillFailure.getFailureDiagnosis().ifPresent(diagnosis -> details.put("diagnosis", diagnosis));
        }
        if (failure instanceof SkillInputValidationException inputFailure) {
            details.put("issues", inputFailure.getIssues());
        }
        return details;
    }
}
