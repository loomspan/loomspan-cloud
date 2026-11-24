package ai.loomspan.cloud.engine;

import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.Part;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;
import tools.jackson.core.StreamReadFeature;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectReader;
import tools.jackson.databind.json.JsonMapper;

import java.io.IOException;

@RestController
final class PublicationController {
    static final String VALIDATE_PATH = "/internal/v1/validate";
    static final String PUBLISH_PATH = "/internal/v1/publish";

    private final PublicationZipAdapter adapter;
    private final PublicationService service;
    private final ObjectReader metadataReader;

    PublicationController(PublicationZipAdapter adapter, PublicationService service, JsonMapper mapper) {
        this.adapter = adapter;
        this.service = service;
        this.metadataReader = mapper.reader().with(StreamReadFeature.STRICT_DUPLICATE_DETECTION)
                .with(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);
    }

    @PostMapping(path = VALIDATE_PATH, consumes = MediaType.MULTIPART_FORM_DATA_VALUE, produces = MediaType.APPLICATION_JSON_VALUE)
    PublicationService.ValidationReport validate(HttpServletRequest request) throws IOException, ServletException {
        var input = readPublication(request, false);
        return service.validate(input.pack(), input.metadata());
    }

    @PostMapping(path = PUBLISH_PATH, consumes = MediaType.MULTIPART_FORM_DATA_VALUE)
    ResponseEntity<Void> publish(HttpServletRequest request) throws IOException, ServletException {
        var input = readPublication(request, true);
        service.publish(input.pack(), input.metadata());
        return ResponseEntity.noContent().build();
    }

    private PublicationInput readPublication(HttpServletRequest request, boolean publishing) throws IOException, ServletException {
        Part pack = null;
        Part metadata = null;
        for (Part part : request.getParts()) {
            switch (part.getName()) {
                case "pack" -> pack = single(pack, part);
                case "metadata" -> metadata = single(metadata, part);
                default -> throw new IllegalArgumentException("Expected only the pack and metadata multipart parts");
            }
        }
        if (metadata == null || metadata.getContentType() == null
                || !MediaType.APPLICATION_JSON.equalsTypeAndSubtype(MediaType.parseMediaType(metadata.getContentType()))) {
            throw new IllegalArgumentException("Expected an application/json multipart part named metadata");
        }
        JsonNode document;
        try (var stream = metadata.getInputStream()) {
            document = metadataReader.readTree(stream);
        }
        var parsed = PublicationMetadata.parse(document, publishing);
        return new PublicationInput(adapter.adapt(parsed.namespace(), pack), parsed);
    }

    private static Part single(Part existing, Part part) {
        if (existing != null) {
            throw new IllegalArgumentException("Duplicate multipart part named " + part.getName());
        }
        return part;
    }

    private record PublicationInput(AdaptedPublication pack, PublicationMetadata metadata) {}
}
