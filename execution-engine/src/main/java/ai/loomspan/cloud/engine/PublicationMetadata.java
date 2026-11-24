package ai.loomspan.cloud.engine;

import org.springframework.boot.convert.DurationStyle;
import org.springframework.util.unit.DataSize;
import tools.jackson.databind.JsonNode;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;

record PublicationMetadata(String namespace, RuntimeSettings settings, String publicationVersion,
                           Map<String, String> variableValues) {
    private static final Pattern NAMESPACE = Pattern.compile("[a-z][a-z0-9]*(?:-[a-z0-9]+)*");
    private static final Pattern NUMBER_WITHOUT_UNIT = Pattern.compile("[+-]?\\d+");
    private static final Pattern SIZE_WITH_UNIT = Pattern.compile("[+-]?\\d+[A-Za-z]+");
    private static final Set<String> TRACE_POLICIES = Set.of("NEVER", "ONERROR", "ALWAYS");
    private static final List<String> QUOTAS = List.of("max-skill-invocations", "max-tool-invocations",
            "max-linter-retries", "max-model-calls", "max-provider-attempts", "max-usage-units");

    static PublicationMetadata parse(JsonNode document, boolean publishing) {
        var root = new Fields(document, "metadata");
        String namespace = root.text("namespace");
        if (namespace.length() > 64 || !NAMESPACE.matcher(namespace).matches()
                || namespace.contains("anthropic") || namespace.contains("claude")) {
            throw invalid("namespace", "must be a valid application namespace");
        }
        var settings = root.object("runtimeSettings");
        var session = settings.object("session");
        String timeout = session.text("mission-timeout");
        requireTimeout(timeout, session.path("mission-timeout"));
        int depth = session.integer("max-depth", 1);
        var attachments = session.object("attachments");
        String maxSize = attachments.text("max-size");
        requireSize(maxSize, attachments.path("max-size"));
        var quotaFields = session.object("quotas");
        var quotas = new LinkedHashMap<String, Integer>();
        QUOTAS.forEach(name -> quotas.put(name, quotaFields.integer(name, 0)));
        var trace = settings.object("execution-trace");
        String persistence = trace.text("persistence");
        if (!TRACE_POLICIES.contains(persistence)) {
            throw invalid(trace.path("persistence"), "must be NEVER, ONERROR, or ALWAYS");
        }
        var effective = new RuntimeSettings(timeout, depth, maxSize, Collections.unmodifiableMap(quotas), persistence);
        if (!publishing) {
            return new PublicationMetadata(namespace, effective, null, Map.of());
        }
        String version = root.text("publicationVersion");
        var supplied = root.object("variableValues");
        var variables = new LinkedHashMap<String, String>();
        supplied.node().properties().forEach(entry -> variables.put(entry.getKey(), supplied.text(entry.getKey())));
        return new PublicationMetadata(namespace, effective, version, Collections.unmodifiableMap(variables));
    }

    private static void requireTimeout(String value, String path) {
        try {
            if (!NUMBER_WITHOUT_UNIT.matcher(value).matches() && DurationStyle.detectAndParse(value).isPositive()) {
                return;
            }
        } catch (IllegalArgumentException | ArithmeticException exception) {
            // Report the metadata path without echoing supplied values.
        }
        throw invalid(path, "must be a positive duration with an explicit unit");
    }

    private static void requireSize(String value, String path) {
        try {
            if (SIZE_WITH_UNIT.matcher(value).matches() && !DataSize.parse(value).isNegative()) {
                return;
            }
        } catch (IllegalArgumentException | ArithmeticException exception) {
            // Report the metadata path without echoing supplied values.
        }
        throw invalid(path, "must be a nonnegative size with an explicit unit");
    }

    private static IllegalArgumentException invalid(String path, String reason) {
        return new IllegalArgumentException(path + " " + reason);
    }

    record RuntimeSettings(String missionTimeout, int maxDepth, String attachmentSize,
                           Map<String, Integer> quotas, String tracePersistence) {}

    private record Fields(JsonNode node, String name) {
        Fields {
            if (node == null || !node.isObject()) {
                throw invalid(name, "must be an object");
            }
        }

        String path(String field) {
            return name.equals("metadata") ? field : name + "." + field;
        }

        Fields object(String field) {
            return new Fields(node.get(field), path(field));
        }

        String text(String field) {
            JsonNode value = node.get(field);
            if (value == null || !value.isString() || value.stringValue().isBlank()) {
                throw invalid(path(field), "must be a nonblank string");
            }
            return value.stringValue();
        }

        int integer(String field, int minimum) {
            JsonNode value = node.get(field);
            if (value == null || !value.isIntegralNumber() || !value.canConvertToInt() || value.intValue() < minimum) {
                throw invalid(path(field), "must be a 32-bit integer of at least " + minimum);
            }
            return value.intValue();
        }
    }
}
