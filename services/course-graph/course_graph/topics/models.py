from typing import Annotated

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

Text = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=600)]


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)


class Source(StrictModel):
    material_id: str
    block_id: int
    start: int
    end: int
    text: str
    page: int | None = None
    week: int | None = None


class Topic(StrictModel):
    id: Text
    title: Text
    requires: list[Text] = Field(max_length=128)
    provides: list[Text] = Field(min_length=1, max_length=128)
    question: Text
    text: str
    sources: list[Source] = Field(min_length=1)
    parent_title: Text | None = None


class Material(StrictModel):
    id: Text
    markdown: str = Field(min_length=1, max_length=2_000_000)
    week: int | None = None
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
    requirements: list[RequirementDecision]
