package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.NamespaceVisibility;
import ai.loomspan.api.PreparedSkillUpdate;
import ai.loomspan.api.SkillCatalog;
import ai.loomspan.api.SkillDocument;
import ai.loomspan.api.SkillReloader;
import ai.loomspan.api.SkillValidationResult;

import java.util.Collection;
import java.util.Map;
import java.util.function.Consumer;

abstract class PublicationReloaderStub implements SkillReloader {
    @Override
    public SkillValidationResult validate(String namespace, Collection<SkillDocument> documents,
                                           ExecutionConfiguration configuration) {
        throw new UnsupportedOperationException();
    }

    @Override
    public SkillValidationResult validate(String namespace, Collection<SkillDocument> documents,
                                           ExecutionConfiguration configuration, NamespaceVisibility visibility) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration, NamespaceVisibility visibility) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration, Map<String, String> variableValues) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration, NamespaceVisibility visibility,
                                       Map<String, String> variableValues) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, String version, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration, NamespaceVisibility visibility) {
        throw new UnsupportedOperationException();
    }

    @Override
    public PreparedSkillUpdate prepare(String namespace, String version, Collection<SkillDocument> documents,
                                       ExecutionConfiguration configuration, NamespaceVisibility visibility,
                                       Map<String, String> variableValues) {
        throw new UnsupportedOperationException();
    }

    @Override
    public void publish(PreparedSkillUpdate update) {
        throw new UnsupportedOperationException();
    }

    @Override
    public void remove(String namespace, String expectedGenerationId) {
        throw new UnsupportedOperationException();
    }

    @Override
    public SkillCatalog snapshot(String namespace) {
        throw new UnsupportedOperationException();
    }

    @Override
    public AutoCloseable onGenerationRetired(Consumer<String> listener) {
        throw new UnsupportedOperationException();
    }
}
