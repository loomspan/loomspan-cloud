package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.SkillValidationIssue;
import com.google.auth.oauth2.GoogleCredentials;
import com.google.auth.oauth2.ServiceAccountCredentials;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import tools.jackson.databind.json.JsonMapper;

import java.net.URI;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyPairGenerator;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class TenantVertexPolicyTest {
    private static final String CONNECTION = "loomspan.connections.tenant";
    private static final String CREDENTIALS = CONNECTION + ".gemini.credentials-json";
    private static final TenantVertexPolicy POLICY = new TenantVertexPolicy();
    private static String serviceAccount;

    @BeforeAll
    static void createSyntheticServiceAccount() throws Exception {
        var privateKey = KeyPairGenerator.getInstance("RSA").generateKeyPair().getPrivate();
        String pem = "-----BEGIN PRIVATE KEY-----\n"
                + Base64.getMimeEncoder(64, new byte[] {'\n'}).encodeToString(privateKey.getEncoded())
                + "\n-----END PRIVATE KEY-----\n";
        serviceAccount = JsonMapper.builder().build().writeValueAsString(Map.of(
                "type", "service_account",
                "client_id", "1234",
                "project_id", "synthetic-test",
                "client_email", "tenant@synthetic-test.example.invalid",
                "private_key_id", "synthetic",
                "private_key", pem,
                "token_uri", URI.create("https://oauth2.example.invalid/token").toString()));
    }

    @Test
    void checksKnownAuthoredCredentialsWithoutResolvingMarkers() {
        assertIssue(vertex(""), CREDENTIALS);
        assertIssue(vertex("        credentials-json: ''\n"), CREDENTIALS);
        assertIssue(vertex("        credentials-json: 12\n"), CREDENTIALS);
        assertIssue(vertex("        credentials-json: malformed\n"), CREDENTIALS);
        assertIssue(vertex("        credentials-json: '{\"type\":\"authorized_user\"}'\n"), CREDENTIALS);
        assertThat(POLICY.checkAuthored(vertex("        credentials-json: { $var: vertex.service-account }\n"))).isEmpty();
        assertThat(POLICY.checkAuthored(vertex("        ? { $var: vertex.field }\n        : malformed\n"))).isEmpty();
        assertThat(POLICY.checkAuthored(vertex(literalCredential()))).isEmpty();
        assertThat(POLICY.checkAuthored(apiKey())).isEmpty();
        var unrelatedMarker = new ExecutionConfiguration(vertex("").yaml() + """
                  executor-config: {milestone: {settings: {$var: 12}}}
                """);
        assertIssue(unrelatedMarker, CREDENTIALS);
    }

    @Test
    void reportsKnownVertexViolationAlongsideMalformedIndependentConnection() {
        var candidate = new ExecutionConfiguration(vertex("").yaml() + """
                    other:
                      driver: openai
                      api-key: first
                      api-key: second
                """);
        assertIssue(candidate, CREDENTIALS);
    }

    @Test
    void reportsKnownVertexViolationAlongsideMalformedFieldsOfTheSameConnection() {
        var malformedHeaders = new ExecutionConfiguration(vertex("").yaml().replace("      gemini:\n", """
                      headers:
                        X-Test: first
                        X-Test: second
                      gemini:
                """));
        assertIssue(malformedHeaders, CREDENTIALS);

        var repeatedProjectId = vertex("        project-id: duplicate\n");
        assertIssue(repeatedProjectId, CREDENTIALS);
    }

    @Test
    void acceptsOnlyExplicitValidServiceAccountJsonInEffectiveVertexMode() {
        assertThatCode(() -> POLICY.requireEffective("milestone", vertex(literalCredential()), Map.of()))
                .doesNotThrowAnyException();
        var supplied = vertex("        credentials-json: { $var: vertex.service-account }\n");
        assertThatCode(() -> POLICY.requireEffective("milestone", supplied, Map.of("vertex.service-account", serviceAccount)))
                .doesNotThrowAnyException();
        for (String value : List.of("  ", "malformed", "{\"type\":\"authorized_user\"}")) {
            assertThatThrownBy(() -> POLICY.requireEffective("milestone", supplied, Map.of("vertex.service-account", value)))
                    .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CREDENTIALS)
                    .hasMessageNotContaining(value.strip().isEmpty() ? "{" : value);
        }
        assertThatThrownBy(() -> POLICY.requireEffective("milestone", supplied, Map.of()))
                .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CREDENTIALS);
        assertThatCode(() -> POLICY.requireEffective("milestone", apiKey(), Map.of())).doesNotThrowAnyException();
    }

    @Test
    void rejectsCredentialTextWithTrailingJson() {
        assertIssue(vertex(literalCredential() + "          {}\n"), CREDENTIALS);
        var supplied = vertex("        credentials-json: { $var: vertex.service-account }\n");
        assertThatThrownBy(() -> POLICY.requireEffective("milestone", supplied,
                Map.of("vertex.service-account", serviceAccount + "\n{}")))
                .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CREDENTIALS);
    }

    @Test
    void verifiesVariableSelectedDriverModeAndFields() {
        var selected = new ExecutionConfiguration("""
                loomspan:
                  connections:
                    tenant:
                      driver: { $var: connection.driver }
                      ? { $var: connection.mode }
                      :
                        ? { $var: gemini.mode }
                        : { $var: vertex.enabled }
                        project-id: synthetic-test
                        location: us-central1
                        ? { $var: gemini.credential-field }
                        : { $var: vertex.service-account }
                """);
        assertThat(POLICY.checkAuthored(selected)).isEmpty();
        var declarations = Map.of("connection.driver", "gemini", "connection.mode", "gemini", "gemini.mode", "vertex-ai",
                "vertex.enabled", "yes", "gemini.credential-field", "credentials-json", "vertex.service-account", serviceAccount);
        assertThatCode(() -> POLICY.requireEffective("milestone", selected, declarations)).doesNotThrowAnyException();

        var missing = new ExecutionConfiguration(selected.yaml()
                .replace("        ? { $var: gemini.credential-field }\n        : { $var: vertex.service-account }\n", ""));
        var withoutCredentials = Map.of("connection.driver", "gemini", "connection.mode", "gemini",
                "gemini.mode", "vertex-ai", "vertex.enabled", "true");
        assertThat(POLICY.checkAuthored(missing)).isEmpty();
        assertThatThrownBy(() -> POLICY.requireEffective("milestone", missing, withoutCredentials))
                .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CREDENTIALS);
    }

    @Test
    void rejectsFileAndUriCredentialsIncludingSubstitutedFieldNames() {
        for (String option : List.of("credentials-file", "credentials-uri")) {
            var authored = vertex("        " + option + ": not-consulted\n");
            assertIssue(authored, CONNECTION + ".gemini." + option);
            assertThatThrownBy(() -> POLICY.requireEffective("milestone", authored, Map.of()))
                    .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CONNECTION + ".gemini." + option);

            var selected = vertex("        ? { $var: vertex.field }\n        : not-consulted\n");
            assertThat(POLICY.checkAuthored(selected)).isEmpty();
            assertThatThrownBy(() -> POLICY.requireEffective("milestone", selected, Map.of("vertex.field", option)))
                    .isInstanceOf(IllegalArgumentException.class).hasMessageContaining(CONNECTION + ".gemini{key#3}");
        }
    }

    @Test
    void rejectsOmittedTenantCredentialsWithHostApplicationDefaultCredentials(@TempDir Path directory) throws Exception {
        Path hostCredentials = Files.writeString(directory.resolve("host-credentials.json"), serviceAccount);
        String classPath = System.getProperty("surefire.test.class.path", System.getProperty("java.class.path"));
        Path arguments = directory.resolve("java.args");
        Files.writeString(arguments, "-cp\n\"" + classPath.replace("\\", "/") + "\"\n"
                + HostCredentialsProbe.class.getName() + "\n");
        String javaName = System.getProperty("os.name").startsWith("Windows") ? "java.exe" : "java";
        var invocation = new ProcessBuilder(Path.of(System.getProperty("java.home"), "bin", javaName).toString(),
                "@" + arguments).redirectErrorStream(true);
        invocation.environment().put("GOOGLE_APPLICATION_CREDENTIALS", hostCredentials.toString());
        var process = invocation.start();
        try {
            assertThat(process.waitFor(20, TimeUnit.SECONDS)).isTrue();
            String output = new String(process.getInputStream().readAllBytes(), java.nio.charset.StandardCharsets.UTF_8);
            assertThat(process.exitValue()).as(output).isZero();
            assertThat(output).contains("Tenant omission rejected despite configured host ADC");
        } finally {
            if (process.isAlive()) {
                process.destroyForcibly();
            }
        }
    }

    public static final class HostCredentialsProbe {
        public static void main(String[] args) throws Exception {
            if (!(GoogleCredentials.getApplicationDefault() instanceof ServiceAccountCredentials)) {
                throw new IllegalStateException("Synthetic host ADC was not loaded");
            }
            try {
                POLICY.requireEffective("milestone", vertex(""), Map.of());
                throw new IllegalStateException("Effective policy accepted a tenant with omitted credentials");
            } catch (IllegalArgumentException expected) {
                if (!expected.getMessage().contains(CREDENTIALS)) {
                    throw expected;
                }
            }
            System.out.println("Tenant omission rejected despite configured host ADC");
        }
    }

    private static ExecutionConfiguration vertex(String added) {
        return new ExecutionConfiguration("""
                loomspan:
                  connections:
                    tenant:
                      driver: Gemini
                      gemini:
                        vertex-ai: true
                        project-id: synthetic-test
                        location: us-central1
                """ + added);
    }

    private static ExecutionConfiguration apiKey() {
        return new ExecutionConfiguration("""
                loomspan:
                  connections:
                    tenant:
                      driver: Gemini
                      api-key: fixture-token
                """);
    }

    private static String literalCredential() {
        return "        credentials-json: |-\n          " + serviceAccount + "\n";
    }

    private static void assertIssue(ExecutionConfiguration config, String path) {
        assertThat(POLICY.checkAuthored(config)).anySatisfy(issue -> {
            assertThat(issue.severity()).isEqualTo(SkillValidationIssue.Severity.ERROR);
            assertThat(issue.sourceName()).isEqualTo(PublicationConfiguration.SOURCE);
            assertThat(issue.fieldPath()).isEqualTo(path);
        });
    }
}
