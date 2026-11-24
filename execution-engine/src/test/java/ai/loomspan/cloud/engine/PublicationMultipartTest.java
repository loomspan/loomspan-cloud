package ai.loomspan.cloud.engine;

import jakarta.servlet.MultipartConfigElement;
import jakarta.servlet.http.Part;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.builder.SpringApplicationBuilder;
import org.springframework.boot.web.server.servlet.context.ServletWebServerApplicationContext;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestPart;
import org.springframework.web.bind.annotation.RestController;

import java.io.ByteArrayInputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

import static org.assertj.core.api.Assertions.assertThat;

class PublicationMultipartTest {
    private static final String BOUNDARY = "loomspan-publication-test";

    @Test
    void enforcesTheIncomingPackPartLimitBeforeDispatch() throws Exception {
        try (var context = new SpringApplicationBuilder(WebApp.class)
                .properties("server.address=127.0.0.1", "server.port=0", "spring.main.banner-mode=off")
                .run();
             var client = HttpClient.newBuilder().version(HttpClient.Version.HTTP_1_1)
                     .connectTimeout(Duration.ofSeconds(5)).build()) {
            var config = context.getBean(MultipartConfigElement.class);
            assertThat(config.getMaxFileSize()).isEqualTo(PublicationZipAdapter.MAX_ZIP_BYTES);
            assertThat(config.getFileSizeThreshold()).isZero();
            var controller = context.getBean(ProbeController.class);
            int port = ((ServletWebServerApplicationContext) context).getWebServer().getPort();
            URI uri = URI.create("http://127.0.0.1:" + port + "/multipart-test");

            byte[] exact = PublicationZipAdapterTest.archiveWithSize(PublicationZipAdapter.MAX_ZIP_BYTES);
            var accepted = send(client, uri, exact);
            assertThat(accepted.statusCode()).isEqualTo(200);
            assertThat(accepted.body()).isEqualTo("loomspan: {}\n");
            assertThat(controller.dispatches()).isEqualTo(1);

            byte[] above = PublicationZipAdapterTest.archiveWithSize(PublicationZipAdapter.MAX_ZIP_BYTES + 1);
            var rejected = send(client, uri, above);
            assertThat(rejected.statusCode()).isEqualTo(413);
            assertThat(controller.dispatches()).isEqualTo(1);
        }
    }

    private static HttpResponse<String> send(HttpClient client, URI uri, byte[] pack) throws Exception {
        String headers = "--" + BOUNDARY + "\r\n"
                + "Content-Disposition: form-data; name=\"metadata\"\r\n"
                + "Content-Type: application/json\r\n\r\n"
                + "{\"namespace\":\"milestone\"}\r\n"
                + "--" + BOUNDARY + "\r\n"
                + "Content-Disposition: form-data; name=\"pack\"; filename=\"publication.zip\"\r\n"
                + "Content-Type: application/zip\r\n\r\n";
        var body = HttpRequest.BodyPublishers.concat(
                HttpRequest.BodyPublishers.ofString(headers),
                HttpRequest.BodyPublishers.ofInputStream(() -> new ByteArrayInputStream(pack)),
                HttpRequest.BodyPublishers.ofString("\r\n--" + BOUNDARY + "--\r\n"));
        assertThat(body.contentLength()).isNegative();
        var request = HttpRequest.newBuilder(uri)
                .timeout(Duration.ofSeconds(10))
                .header("Content-Type", "multipart/form-data; boundary=" + BOUNDARY)
                .POST(body)
                .build();
        return client.send(request, HttpResponse.BodyHandlers.ofString());
    }

    @Configuration(proxyBeanMethods = false)
    @EnableAutoConfiguration(excludeName = {
            "ai.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanAiAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanObservabilityWebAutoConfiguration"
    })
    @Import(ProbeController.class)
    static class WebApp {}

    @RestController
    static class ProbeController {
        private final PublicationZipAdapter adapter = new PublicationZipAdapter();
        private final java.util.concurrent.atomic.AtomicInteger dispatches = new java.util.concurrent.atomic.AtomicInteger();

        @PostMapping(path = "/multipart-test", consumes = MediaType.MULTIPART_FORM_DATA_VALUE)
        String adapt(@RequestPart("pack") Part pack) {
            dispatches.incrementAndGet();
            return adapter.adapt("milestone", pack).executionYaml();
        }

        int dispatches() {
            return dispatches.get();
        }
    }
}
