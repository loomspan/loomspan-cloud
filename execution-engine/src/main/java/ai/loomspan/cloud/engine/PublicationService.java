package ai.loomspan.cloud.engine;

import ai.loomspan.api.NamespaceVisibility;
import ai.loomspan.api.SkillKind;
import ai.loomspan.api.SkillReloader;
import ai.loomspan.api.SkillValidationIssue;
import ai.loomspan.api.ValidatedSkill;
import com.fasterxml.jackson.annotation.JsonInclude;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

@Service
final class PublicationService {
    private final SkillReloader reloader;
    private final TenantVertexPolicy vertexPolicy;

    PublicationService(SkillReloader reloader, TenantVertexPolicy vertexPolicy) {
        this.reloader = reloader;
        this.vertexPolicy = vertexPolicy;
    }

    ValidationReport validate(AdaptedPublication pack, PublicationMetadata metadata) {
        var configuration = PublicationConfiguration.compose(pack.executionYaml(), metadata.settings());
        var authoredIssues = vertexPolicy.checkAuthored(configuration);
        var checked = reloader.validate(metadata.namespace(), pack.skills(), configuration, NamespaceVisibility.PRIVATE);
        var issues = new ArrayList<>(authoredIssues);
        issues.addAll(checked.issues());
        boolean valid = issues.stream().noneMatch(issue -> issue.severity() == SkillValidationIssue.Severity.ERROR);
        return new ValidationReport(valid, issues.stream().map(ValidationIssue::from).toList(),
                valid ? checked.skills().stream().map(CheckedSkill::from).toList() : List.of());
    }

    void publish(AdaptedPublication pack, PublicationMetadata metadata) {
        var configuration = PublicationConfiguration.compose(pack.executionYaml(), metadata.settings());
        vertexPolicy.requireEffective(metadata.namespace(), configuration, metadata.variableValues());
        try (var prepared = reloader.prepare(metadata.namespace(), metadata.publicationVersion(), pack.skills(),
                configuration, NamespaceVisibility.PRIVATE, metadata.variableValues())) {
            reloader.publish(prepared);
        }
    }

    record ValidationReport(boolean valid, List<ValidationIssue> issues, List<CheckedSkill> skills) {}

    @JsonInclude(JsonInclude.Include.NON_NULL)
    record ValidationIssue(SkillValidationIssue.Severity severity, String sourceName, String message,
                           String skillName, String fieldPath) {
        static ValidationIssue from(SkillValidationIssue issue) {
            String source = "<configuration>".equals(issue.sourceName())
                    ? PublicationConfiguration.SOURCE : issue.sourceName();
            return new ValidationIssue(issue.severity(), source, issue.message(), issue.skillName(), issue.fieldPath());
        }
    }

    record CheckedSkill(String namespace, String localName, SkillKind kind) {
        static CheckedSkill from(ValidatedSkill skill) {
            return new CheckedSkill(skill.namespace(), skill.localName(), skill.kind());
        }
    }
}
