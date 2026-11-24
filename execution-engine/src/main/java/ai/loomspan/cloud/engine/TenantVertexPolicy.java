package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.SkillValidationIssue;
import ai.loomspan.autoconfigure.AiDriver;
import ai.loomspan.internal.serialization.AuthoredYamlVariables;
import ai.loomspan.internal.serialization.AuthoredYamlVariables.Slice;
import ai.loomspan.internal.skill.ExecutionConfigurationParser;
import com.google.auth.oauth2.ServiceAccountCredentials;
import org.snakeyaml.engine.v2.exceptions.YamlEngineException;
import org.springframework.boot.convert.ApplicationConversionService;
import org.springframework.core.convert.ConversionException;
import org.springframework.stereotype.Component;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.ObjectReader;
import tools.jackson.databind.json.JsonMapper;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Component
final class TenantVertexPolicy {
    private static final String CONNECTIONS = "loomspan.connections.";
    private static final String CREDENTIALS = "credentials-json";
    private static final List<String> FORBIDDEN_OPTIONS = List.of("credentials-file", "credentials-uri");
    private static final ObjectReader CREDENTIAL_JSON = JsonMapper.builder().build().reader()
            .with(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);

    List<SkillValidationIssue> checkAuthored(ExecutionConfiguration configuration) {
        List<Map.Entry<Slice, Slice>> entries;
        try {
            var connections = document(configuration).slice("loomspan", "connections");
            entries = connections == null ? null : connections.entries();
        } catch (IllegalArgumentException | YamlEngineException exception) {
            // Framework owns structural diagnostics for an unreadable configuration.
            return List.of();
        }
        if (entries == null) {
            return List.of();
        }
        var issues = new ArrayList<SkillValidationIssue>();
        for (var entry : entries) {
            try {
                if (entry.getKey().literal() instanceof String) {
                    checkAuthoredConnection(entry.getValue(), issues);
                }
            } catch (IllegalArgumentException | YamlEngineException exception) {
                // Framework reports the malformed connection. Continue checking independent entries.
            }
        }
        return List.copyOf(issues);
    }

    private static void checkAuthoredConnection(Slice connection, List<SkillValidationIssue> issues) {
        var fields = AuthoredFields.read(connection);
        if (fields == null) {
            return;
        }
        var gemini = AuthoredFields.read(fields.values().get("gemini"));
        if (gemini == null) {
            return;
        }
        String path = connection.location().path() + ".gemini.";
        for (String option : FORBIDDEN_OPTIONS) {
            if (gemini.values().containsKey(option)) {
                issues.add(issue(path + option, "is not permitted for tenant credentials"));
            }
        }
        if (converted(fields.literal("driver"), AiDriver.class) != AiDriver.GEMINI
                || !Boolean.TRUE.equals(converted(gemini.literal("vertex-ai"), Boolean.class))) {
            return;
        }
        if (gemini.isUndecidable(CREDENTIALS)) {
            return;
        }
        String failure = credentialFailure(gemini.literal(CREDENTIALS));
        if (failure != null) {
            issues.add(issue(path + CREDENTIALS, failure));
        }
    }

    private record AuthoredFields(Map<String, Slice> values, boolean hasUnresolvedKey) {
        static AuthoredFields read(Slice mapping) {
            var entries = mapping == null ? null : mapping.entries();
            if (entries == null) {
                return null;
            }
            var fields = new LinkedHashMap<String, Slice>();
            boolean unresolvedKey = false;
            for (var entry : entries) {
                var key = entry.getKey();
                if (key.variable()) {
                    unresolvedKey = true;
                    continue;
                }
                try {
                    if (key.literal() instanceof String name) {
                        // Defer ambiguous repeated policy fields to Framework's structural check.
                        fields.put(name, fields.containsKey(name) ? null : entry.getValue());
                    }
                } catch (IllegalArgumentException | YamlEngineException exception) {
                    // An unreadable key cannot name any known policy field.
                }
            }
            return new AuthoredFields(fields, unresolvedKey);
        }

        Object literal(String name) {
            var value = values.get(name);
            return value == null ? null : value.literal();
        }

        boolean isUndecidable(String name) {
            var value = values.get(name);
            return value == null ? hasUnresolvedKey || values.containsKey(name) : value.variable();
        }
    }

    void requireEffective(String namespace, ExecutionConfiguration configuration, Map<String, String> values) {
        // This internal Framework parser has no runtime factory. Final preparation still receives
        // the full explicit map so skill-only references and unused names are checked by Framework.
        var executionValues = new LinkedHashMap<String, String>();
        for (var occurrence : document(configuration).discover()) {
            String value = values.get(occurrence.name());
            if (value != null) {
                executionValues.put(occurrence.name(), value);
            }
        }
        var effective = ExecutionConfigurationParser.parse(namespace, configuration, executionValues);
        effective.properties().getConnections().forEach((name, connection) -> {
            var gemini = connection.getGemini();
            if (connection.getDriver() == AiDriver.GEMINI && gemini != null && Boolean.TRUE.equals(gemini.getVertexAi())) {
                String failure = credentialFailure(gemini.getCredentialsJson());
                if (failure != null) {
                    throw new IllegalArgumentException(CONNECTIONS + name + ".gemini." + CREDENTIALS + " " + failure);
                }
            }
        });
    }

    private static AuthoredYamlVariables.Document document(ExecutionConfiguration configuration) {
        return AuthoredYamlVariables.execution(configuration.yaml(), PublicationConfiguration.SOURCE);
    }

    private static <T> T converted(Object value, Class<T> target) {
        if (!(value instanceof String || value instanceof Boolean || value instanceof Number)) {
            return null;
        }
        try {
            return ApplicationConversionService.getSharedInstance().convert(value.toString(), target);
        } catch (ConversionException exception) {
            return null;
        }
    }

    private static String credentialFailure(Object credentials) {
        if (!(credentials instanceof String json) || json.isBlank()) {
            return "must provide nonblank service-account JSON for tenant Vertex AI connections";
        }
        byte[] encoded = json.getBytes(StandardCharsets.UTF_8);
        try (var input = new ByteArrayInputStream(encoded)) {
            // The credential loader may stop reading at the end of its first object.
            CREDENTIAL_JSON.readTree(encoded);
            ServiceAccountCredentials.fromStream(input);
            return null;
        } catch (IOException | RuntimeException exception) {
            return "must provide valid service-account JSON for tenant Vertex AI connections";
        }
    }

    private static SkillValidationIssue issue(String path, String failure) {
        return new SkillValidationIssue(SkillValidationIssue.Severity.ERROR, PublicationConfiguration.SOURCE,
                null, path, path + " " + failure);
    }
}
