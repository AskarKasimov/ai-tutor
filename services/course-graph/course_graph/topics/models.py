from typing import Annotated

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

Text = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=600)]


# Most requires and provides of one topic card, shared with the graph contract.
TOPIC_LIST_MAX = 128


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)


class Source(StrictModel):
    material_id: str
    block_id: int
    start: int
    end: int
    text: str
    page: int | None = None
    week: int | None = Field(default=None, gt=0)
    kind: str | None = None
    filename: str | None = None
    kind_label: str | None = None
    material_revision: str | None = None


class Topic(StrictModel):
    id: Text
    title: Text
    requires: list[Text] = Field(max_length=TOPIC_LIST_MAX)
    provides: list[Text] = Field(min_length=1, max_length=TOPIC_LIST_MAX)
    question: Text
    text: str
    sources: list[Source] = Field(min_length=1)
    parent_title: Text | None = None


class Material(StrictModel):
    id: Text
    markdown: str = Field(min_length=1, max_length=2_000_000)
    week: int | None = None
    kind: str | None = None
    filename: str | None = None
    kind_label: str | None = None
    material_revision: str | None = None
    topics: list[Topic] | None = None


class Removal(StrictModel):
    id: int
    reason: str


class TopicDecision(StrictModel):
    title: Text
    requires: list[Text] = Field(max_length=12)
    provides: list[Text] = Field(min_length=1, max_length=12)
    question: Text
    block_ids: list[int] = Field(min_length=1)
    parent_title: Text | None = None


class ExtractionDecision(StrictModel):
    remove: list[Removal]
    topics: list[TopicDecision]


class Evidence(StrictModel):
    material_id: Text
    block_id: int
    quote: Text


class LinkDecision(StrictModel):
    target_id: Text
    prerequisite_id: Text
    requirement: Text
    reason: Text
    target_evidence: Evidence
    prerequisite_evidence: Evidence


class RequirementDecision(StrictModel):
    target_id: Text
    requirement: Text
    prerequisite_ids: list[Text]
    reason: Text


class LinkResponse(StrictModel):
    links: list[LinkDecision]
    # Decisions explain the links; the accepted links themselves are the result.
    requirements: list[RequirementDecision] = Field(default_factory=list)


class SynonymDecision(StrictModel):
    a: Text
    b: Text
    same: bool


class SynonymResponse(StrictModel):
    pairs: list[SynonymDecision]


class AttachLink(StrictModel):
    id: Text
    reason: Text
    topic_evidence: Evidence
    candidate_evidence: Evidence


class AttachDecision(StrictModel):
    id: Text
    prerequisites: list[AttachLink] = Field(default_factory=list)
    dependents: list[AttachLink] = Field(default_factory=list)


class AttachResponse(StrictModel):
    topics: list[AttachDecision]


class TopicQuestion(StrictModel):
    id: Text
    question: Text


class QuestionResponse(StrictModel):
    questions: list[TopicQuestion]
