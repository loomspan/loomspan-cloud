package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import org.snakeyaml.engine.v2.api.DumpSettings;
import org.snakeyaml.engine.v2.api.lowlevel.Compose;
import org.snakeyaml.engine.v2.api.lowlevel.Present;
import org.snakeyaml.engine.v2.api.lowlevel.Serialize;
import org.snakeyaml.engine.v2.common.FlowStyle;
import org.snakeyaml.engine.v2.common.ScalarStyle;
import org.snakeyaml.engine.v2.nodes.MappingNode;
import org.snakeyaml.engine.v2.nodes.Node;
import org.snakeyaml.engine.v2.nodes.NodeTuple;
import org.snakeyaml.engine.v2.nodes.ScalarNode;
import org.snakeyaml.engine.v2.nodes.Tag;
import org.snakeyaml.engine.v2.schema.CoreSchema;

import java.util.ArrayList;
import java.util.List;

final class PublicationConfiguration {
    static final String SOURCE = PublicationZipAdapter.EXECUTION_PATH;
    private static final DumpSettings DUMP = DumpSettings.builder().setSchema(new CoreSchema()).build();

    private PublicationConfiguration() {}

    static ExecutionConfiguration compose(String yaml, PublicationMetadata.RuntimeSettings settings) {
        Node root = new Compose(PublicationZipAdapter.YAML_SETTINGS).composeString(yaml).orElse(null);
        if (!(root instanceof MappingNode document)) {
            return new ExecutionConfiguration(yaml);
        }
        var fields = document.getValue().stream().filter(field -> isLoomspanKey(field.getKeyNode())).toList();
        if (fields.size() != 1 || !(fields.getFirst().getValueNode() instanceof MappingNode loomspan)) {
            return new ExecutionConfiguration(yaml);
        }

        var quotas = new ArrayList<NodeTuple>();
        settings.quotas().forEach((field, value) -> quotas.add(field(field, integer(value))));
        var session = new MappingNode(Tag.MAP, List.of(
                field("mission-timeout", text(settings.missionTimeout())),
                field("max-depth", integer(settings.maxDepth())),
                field("attachments", mapping(field("max-size", text(settings.attachmentSize())))),
                field("quotas", new MappingNode(Tag.MAP, quotas, FlowStyle.BLOCK))), FlowStyle.BLOCK);
        var trace = mapping(field("persistence", text(settings.tracePersistence())));

        // Keep the authored graph so aliases and mapping-valued keys survive composition.
        var effective = new ArrayList<>(loomspan.getValue());
        effective.add(field("session", session));
        effective.add(field("execution-trace", trace));
        loomspan.setValue(effective);
        return new ExecutionConfiguration(new Present(DUMP).emitToString(new Serialize(DUMP).serializeOne(root).iterator()));
    }

    private static boolean isLoomspanKey(Node node) {
        return node instanceof ScalarNode scalar && Tag.STR.equals(scalar.getTag()) && "loomspan".equals(scalar.getValue());
    }

    private static MappingNode mapping(NodeTuple field) {
        return new MappingNode(Tag.MAP, List.of(field), FlowStyle.BLOCK);
    }

    private static NodeTuple field(String key, Node value) {
        return new NodeTuple(new ScalarNode(Tag.STR, key, ScalarStyle.PLAIN), value);
    }

    private static ScalarNode text(String value) {
        return new ScalarNode(Tag.STR, value, ScalarStyle.DOUBLE_QUOTED);
    }

    private static ScalarNode integer(int value) {
        return new ScalarNode(Tag.INT, Integer.toString(value), ScalarStyle.PLAIN);
    }
}
