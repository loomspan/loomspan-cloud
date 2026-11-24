package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillDocument;

import java.util.List;

record AdaptedPublication(String executionYaml, List<SkillDocument> skills) {
    AdaptedPublication {
        skills = List.copyOf(skills);
    }
}
