package ai.loomspan.cloud.engine;

import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.builder.SpringApplicationBuilder;
import org.springframework.boot.web.server.servlet.context.ServletWebServerApplicationContext;
import org.springframework.boot.web.servlet.ServletRegistrationBean;
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import tools.jackson.databind.json.JsonMapper;

import javax.net.ssl.KeyManagerFactory;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManagerFactory;
import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.security.KeyStore;
import java.security.PrivateKey;
import java.security.cert.Certificate;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class EngineClientIdentityTest {
    private static final String KEY = "fixture-observability-key-for-auth-tests";
    private static final String KEY_HEADER = "X-loomspan-Api-Key";
    private static final String INSTANCE_HEADER = "X-loomspan-Instance-Id";
    private static final String INTERNAL = "/internal/v1/auth-probe";
    private static final String OBSERVABILITY = "/_loomspan/observability/v1";
    private static final String INSTANCE = OBSERVABILITY + "/instance";
    private static final String ENGINE_SANS = "SAN=ip:127.0.0.1,dns:execution-engine.loomspan-engine.svc";
    private static final String MANAGEMENT_ID = EngineClientIdentityConfiguration.MANAGEMENT_CLIENT_IDENTITY;
    private static final String STORE_PASSWORD = "fixture-password";
    private static final char[] PASSWORD = STORE_PASSWORD.toCharArray();
    private static final JsonMapper JSON = JsonMapper.builder().build();

    private Path temporary;
    private Authority issuer;
    private Leaf server;
    private Leaf authorized;
    private Leaf wrongIdentity;
    private Leaf commonNameOnly;
    private Leaf untrusted;
    private Path serverCertificate;
    private Path serverKey;
    private URI base;
    private ConfigurableApplicationContext context;

    @BeforeAll
    void start(@TempDir Path testDirectory) throws Exception {
        temporary = testDirectory;
        issuer = authority(temporary, "fixture-ca");
        var anotherIssuer = authority(temporary, "untrusted-fixture-ca");
        server = leaf(temporary, issuer, "server", "engine", "serverAuth", ENGINE_SANS);
        authorized = leaf(temporary, issuer, "authorized", "management", "clientAuth", "SAN=uri:" + MANAGEMENT_ID);
        wrongIdentity = leaf(temporary, issuer, "wrong-identity", "other-client", "clientAuth",
                "SAN=uri:spiffe://loomspan-cloud/management/other");
        commonNameOnly = leaf(temporary, issuer, "common-name-only", MANAGEMENT_ID, "clientAuth", null);
        untrusted = leaf(temporary, anotherIssuer, "untrusted", "untrusted", "clientAuth", "SAN=uri:" + MANAGEMENT_ID);
        serverCertificate = Files.copy(server.pem(), temporary.resolve("mounted-tls.crt"));
        serverKey = temporary.resolve("mounted-tls.key");
        writeKey(serverKey, server.key());
        context = new SpringApplicationBuilder(WebApp.class).profiles("k3s").run(
                "--server.port=0", "--server.address=127.0.0.1", "--server.shutdown=immediate",
                "--spring.main.banner-mode=off",
                "--LOOMSPAN_ENGINE_CERTIFICATE=" + serverCertificate.toUri(),
                "--LOOMSPAN_ENGINE_PRIVATE_KEY=" + serverKey.toUri(),
                "--LOOMSPAN_ENGINE_TRUST_CERTIFICATE=" + issuer.pem().toUri(),
                "--LOOMSPAN_CONSOLE_API_KEY=" + KEY);
        int port = ((ServletWebServerApplicationContext) context).getWebServer().getPort();
        base = URI.create("https://127.0.0.1:" + port);
    }

    @AfterAll
    void stop() {
        if (context != null) {
            context.close();
        }
    }

    @Test
    void authenticatedLeafAndConsoleKeyAreRequiredIndependently() throws Exception {
        var probe = context.getBean(InternalProbe.class);
        int before = probe.dispatches.get();
        try (var good = client(authorized);
             var wrong = client(wrongIdentity);
             var cnOnly = client(commonNameOnly);
             var missing = client(null);
             var otherIssuer = client(untrusted)) {
            assertThat(get(good, INTERNAL, null).statusCode()).isEqualTo(204);
            assertThat(get(wrong, INTERNAL, null).statusCode()).isEqualTo(403);
            assertThat(get(cnOnly, INTERNAL, null).statusCode()).isEqualTo(403);
            for (var invalid : List.of(missing, otherIssuer)) {
                assertThatThrownBy(() -> get(invalid, INTERNAL, null)).isInstanceOf(IOException.class);
                assertThatThrownBy(() -> get(invalid, INSTANCE, KEY)).isInstanceOf(IOException.class);
            }
            assertThat(probe.dispatches.get()).isEqualTo(before + 1);

            for (var insufficient : List.of(wrong, cnOnly)) {
                var rejected = get(insufficient, INSTANCE, KEY);
                assertThat(rejected.statusCode()).isEqualTo(403);
                assertThat(rejected.headers().firstValue(INSTANCE_HEADER)).isEmpty();
            }
            for (String invalidKey : new String[]{null, "too-short", KEY + "invalid"}) {
                var rejected = get(good, INSTANCE, invalidKey);
                assertThat(rejected.statusCode()).isEqualTo(401);
                assertThat(rejected.headers().firstValue(INSTANCE_HEADER)).isEmpty();
            }
            var snapshot = get(good, INSTANCE, KEY);
            assertThat(snapshot.statusCode()).isEqualTo(200);
            var parsed = JSON.readTree(snapshot.body());
            String instance = parsed.get("instanceId").asString();
            assertThat(instance).matches("[0-9a-f-]{36}");
            assertThat(snapshot.headers().firstValue(INSTANCE_HEADER)).contains(instance);
            assertThat(parsed.get("namespaces")).allMatch(namespace -> namespace.get("publicationVersion").isNull());
            assertStreamHandshake(good, instance);
        }
    }

    @Test
    void enforcesCertificateIdentityAcrossProtectedDispatches() throws Exception {
        var probe = context.getBean(InternalProbe.class);
        int before = probe.dispatches.get();
        try (var good = client(authorized); var wrong = client(wrongIdentity)) {
            for (String path : List.of("/forward-probe", "/async-probe")) {
                assertThat(get(wrong, path, null).statusCode()).isEqualTo(403);
                assertThat(get(good, path, null).statusCode()).isEqualTo(204);
            }
        }
        assertThat(probe.dispatches.get()).isEqualTo(before + 2);
    }

    @Test
    void renewedServerCertificateReachesFreshHandshakesWithoutRestart() throws Exception {
        String instance;
        try (var good = client(authorized)) {
            instance = get(good, INSTANCE, KEY).headers().firstValue(INSTANCE_HEADER).orElseThrow();
        }
        var replacement = leaf(temporary, issuer, "replacement-server", "engine", "serverAuth", ENGINE_SANS);
        Path replacementKey = temporary.resolve("replacement.key");
        writeKey(replacementKey, replacement.key());
        Files.move(replacementKey, serverKey, StandardCopyOption.REPLACE_EXISTING);
        Files.move(replacement.pem(), serverCertificate, StandardCopyOption.REPLACE_EXISTING);

        long deadline = System.nanoTime() + Duration.ofSeconds(20).toNanos();
        do {
            try (var good = client(authorized)) {
                var response = get(good, INSTANCE, KEY);
                var peer = (X509Certificate) response.sslSession().orElseThrow().getPeerCertificates()[0];
                if (peer.equals(replacement.certificate())) {
                    assertThat(response.statusCode()).isEqualTo(200);
                    assertThat(response.headers().firstValue(INSTANCE_HEADER)).contains(instance);
                    return;
                }
            }
            Thread.sleep(100);
        } while (System.nanoTime() < deadline);
        throw new AssertionError("new TLS sessions never received the renewed server certificate");
    }

    private void assertStreamHandshake(HttpClient client, String instance) throws Exception {
        var baseline = get(client, OBSERVABILITY + "/active-executions?pageSize=1", KEY);
        assertThat(baseline.statusCode()).isEqualTo(200);
        String cursor = JSON.readTree(baseline.body()).get("resumeCursor").asString();
        var request = HttpRequest.newBuilder(base.resolve(OBSERVABILITY + "/activity?instanceId=" + instance
                        + "&afterCursor=" + cursor)).timeout(Duration.ofSeconds(5))
                .header(KEY_HEADER, KEY).header("Accept", "text/event-stream").GET().build();
        var response = client.send(request, HttpResponse.BodyHandlers.ofInputStream());
        try (var reader = new BufferedReader(new InputStreamReader(response.body(), StandardCharsets.UTF_8))) {
            assertThat(response.statusCode()).isEqualTo(200);
            assertThat(response.headers().firstValue(INSTANCE_HEADER)).contains(instance);
            var lines = CompletableFuture.supplyAsync(() -> {
                try {
                    return List.of(reader.readLine(), reader.readLine());
                } catch (IOException ex) {
                    throw new java.io.UncheckedIOException(ex);
                }
            }).get(5, TimeUnit.SECONDS);
            assertThat(lines.get(0)).isEqualTo("event: handshake");
            assertThat(lines.get(1)).startsWith("data: ").contains(instance);
        }
    }

    private HttpResponse<String> get(HttpClient client, String path, String key) throws Exception {
        var request = HttpRequest.newBuilder(base.resolve(path)).timeout(Duration.ofSeconds(5))
                .header("X-Forwarded-Proto", "https")
                .header("X-Forwarded-Client-Cert", "URI=" + MANAGEMENT_ID);
        if (key != null) {
            request.header(KEY_HEADER, key);
        }
        return client.send(request.GET().build(), HttpResponse.BodyHandlers.ofString());
    }

    private HttpClient client(Leaf leaf) throws Exception {
        return client(issuer, leaf);
    }

    static AuthorizedFixture authorizedFixture(Path temporary) throws Exception {
        var issuer = authority(temporary, "fixture-ca");
        var server = leaf(temporary, issuer, "server", "engine", "serverAuth", ENGINE_SANS);
        var management = leaf(temporary, issuer, "authorized", "management", "clientAuth", "SAN=uri:" + MANAGEMENT_ID);
        var serverKey = temporary.resolve("mounted-tls.key");
        writeKey(serverKey, server.key());
        return new AuthorizedFixture(issuer, management, server.pem(), serverKey);
    }

    record AuthorizedFixture(Authority issuer, Leaf management, Path serverPem, Path serverKey) {
        HttpClient openClient() throws Exception {
            return client(issuer, management);
        }

        String[] launchArguments() {
            return new String[]{"--server.port=0", "--server.address=127.0.0.1", "--server.shutdown=immediate",
                    "--spring.main.banner-mode=off", "--LOOMSPAN_ENGINE_CERTIFICATE=" + serverPem.toUri(),
                    "--LOOMSPAN_ENGINE_PRIVATE_KEY=" + serverKey.toUri(),
                    "--LOOMSPAN_ENGINE_TRUST_CERTIFICATE=" + issuer.pem().toUri(), "--LOOMSPAN_CONSOLE_API_KEY=" + KEY};
        }
    }

    private static HttpClient client(Authority issuer, Leaf leaf) throws Exception {
        var trustStore = KeyStore.getInstance("PKCS12");
        trustStore.load(null, PASSWORD);
        trustStore.setCertificateEntry("issuer", issuer.certificate());
        var trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        trust.init(trustStore);

        var keys = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm());
        var keyStore = KeyStore.getInstance("PKCS12");
        keyStore.load(null, PASSWORD);
        if (leaf != null) {
            keyStore.setKeyEntry("client", leaf.key(), PASSWORD,
                    new Certificate[]{leaf.certificate(), leaf.authority().certificate()});
        }
        keys.init(keyStore, PASSWORD);
        var ssl = SSLContext.getInstance("TLS");
        ssl.init(keys.getKeyManagers(), trust.getTrustManagers(), null);
        return HttpClient.newBuilder().version(HttpClient.Version.HTTP_1_1)
                .sslContext(ssl).connectTimeout(Duration.ofSeconds(5)).build();
    }

    private static Authority authority(Path temporary, String name) throws Exception {
        Path store = temporary.resolve(name + ".p12");
        Path pem = temporary.resolve(name + ".pem");
        keytool("-genkeypair", "-keystore", store.toString(), "-alias", name,
                "-dname", "CN=" + name, "-keyalg", "EC", "-validity", "3", "-ext", "BC=ca:true",
                "-ext", "KU=keyCertSign,cRLSign");
        keytool("-exportcert", "-rfc", "-keystore", store.toString(), "-alias", name, "-file", pem.toString());
        return new Authority(name, store, pem, certificate(pem));
    }

    private static Leaf leaf(Path temporary, Authority authority, String alias, String commonName, String usage, String extensions)
            throws Exception {
        Path store = temporary.resolve(alias + ".p12");
        Path csr = temporary.resolve(alias + ".csr");
        Path pem = temporary.resolve(alias + ".pem");
        keytool("-genkeypair", "-keystore", store.toString(), "-alias", alias, "-dname", "CN=" + commonName,
                "-keyalg", "EC", "-validity", "2");
        keytool("-certreq", "-keystore", store.toString(), "-alias", alias, "-file", csr.toString());
        var signing = new ArrayList<>(List.of("-gencert", "-rfc", "-keystore", authority.store().toString(),
                "-alias", authority.alias(), "-infile", csr.toString(), "-outfile", pem.toString(),
                "-validity", "2", "-ext", "EKU=" + usage, "-ext", "KU=digitalSignature"));
        if (extensions != null) {
            signing.addAll(List.of("-ext", extensions));
        }
        keytool(signing.toArray(String[]::new));
        var keys = KeyStore.getInstance(store.toFile(), PASSWORD);
        return new Leaf(authority, pem, certificate(pem), (PrivateKey) keys.getKey(alias, PASSWORD));
    }

    private static X509Certificate certificate(Path file) throws Exception {
        try (var stream = Files.newInputStream(file)) {
            return (X509Certificate) CertificateFactory.getInstance("X.509").generateCertificate(stream);
        }
    }

    private static void writeKey(Path file, PrivateKey key) throws IOException {
        var encoded = Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(key.getEncoded());
        Files.writeString(file, "-----BEGIN PRIVATE KEY-----\n" + encoded + "\n-----END PRIVATE KEY-----\n");
    }

    private static void keytool(String... arguments) throws Exception {
        String executable = System.getProperty("os.name").startsWith("Windows") ? "keytool.exe" : "keytool";
        var command = new ArrayList<>(List.of(Path.of(System.getProperty("java.home"), "bin", executable).toString()));
        command.addAll(List.of(arguments));
        command.addAll(List.of("-storetype", "PKCS12", "-storepass", STORE_PASSWORD));
        var process = new ProcessBuilder(command).redirectErrorStream(true).start();
        if (!process.waitFor(15, TimeUnit.SECONDS)) {
            process.destroyForcibly();
            throw new AssertionError("keytool fixture generation timed out");
        }
        if (process.exitValue() != 0) {
            throw new AssertionError(new String(process.getInputStream().readAllBytes(), StandardCharsets.UTF_8));
        }
    }

    private record Authority(String alias, Path store, Path pem, X509Certificate certificate) {}

    private record Leaf(Authority authority, Path pem, X509Certificate certificate, PrivateKey key) {}

    @Configuration(proxyBeanMethods = false)
    @EnableAutoConfiguration
    @Import({EngineClientIdentityConfiguration.class, InternalProbe.class})
    static class WebApp {
        @Bean
        ServletRegistrationBean<HttpServlet> relays() {
            HttpServlet relay = new HttpServlet() {
                @Override
                protected void doGet(HttpServletRequest request, HttpServletResponse response)
                        throws jakarta.servlet.ServletException, IOException {
                    if (request.getServletPath().equals("/forward-probe")) {
                        request.getRequestDispatcher(INTERNAL).forward(request, response);
                    } else {
                        request.startAsync().dispatch(INTERNAL);
                    }
                }
            };
            var registration = new ServletRegistrationBean<>(relay, "/forward-probe", "/async-probe");
            registration.setAsyncSupported(true);
            return registration;
        }
    }

    @RestController
    static class InternalProbe {
        final AtomicInteger dispatches = new AtomicInteger();

        @GetMapping(INTERNAL)
        ResponseEntity<Void> hit() {
            dispatches.incrementAndGet();
            return ResponseEntity.noContent().build();
        }
    }
}
