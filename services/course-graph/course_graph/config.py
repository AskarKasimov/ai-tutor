"""Strict process configuration loaded once from a service-local YAML file."""

from __future__ import annotations

import os
import re
from functools import lru_cache
from pathlib import Path
from typing import Annotated

import yaml
from pydantic import BaseModel, ConfigDict, Field, StrictBool, StrictInt, StrictStr, field_validator


def _duration_seconds(value: str) -> float:
    match = re.fullmatch(r"(\d+(?:\.\d+)?)(ms|s|m|h)", value)
    if not match:
        raise ValueError("duration must use ms, s, m, or h")
    amount = float(match.group(1))
    return amount * {"ms": 0.001, "s": 1, "m": 60, "h": 3600}[match.group(2)]


class SettingsModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class ServerSettings(SettingsModel):
    port: Annotated[StrictInt, Field(ge=1, le=65535)]


class GatewaySettings(SettingsModel):
    address: StrictStr = Field(min_length=1)
    timeout: StrictStr

    @field_validator("timeout")
    @classmethod
    def valid_timeout(cls, value: str) -> str:
        if _duration_seconds(value) <= 0:
            raise ValueError("timeout must be positive")
        return value

    @property
    def timeout_seconds(self) -> float:
        return _duration_seconds(self.timeout)


class EmbeddingSettings(SettingsModel):
    model: StrictStr = Field(min_length=1)
    threads: Annotated[StrictInt, Field(ge=1, le=64)]


class TopicSettings(SettingsModel):
    max_chars: Annotated[StrictInt, Field(ge=1500, le=20000)]


class BootstrapSettings(SettingsModel):
    enabled: StrictBool
    attempts: Annotated[StrictInt, Field(ge=1, le=5)]


class ModelSettings(SettingsModel):
    bootstrap: BootstrapSettings


class OCRSettings(SettingsModel):
    threads: Annotated[StrictInt, Field(ge=1, le=64)]
    document_timeout: StrictStr
    enrich_code: StrictBool
    enrich_formula: StrictBool
    picture_text: StrictBool
    full_page: StrictBool

    @field_validator("document_timeout")
    @classmethod
    def valid_document_timeout(cls, value: str) -> str:
        if _duration_seconds(value) < 0:
            raise ValueError("document_timeout must not be negative")
        return value

    @property
    def document_timeout_seconds(self) -> float:
        return _duration_seconds(self.document_timeout)


class CacheSettings(SettingsModel):
    documents_dir: StrictStr = Field(min_length=1)
    rapidocr_dir: StrictStr = Field(min_length=1)


class Settings(SettingsModel):
    server: ServerSettings
    llm_gateway: GatewaySettings
    embedding: EmbeddingSettings
    topics: TopicSettings
    models: ModelSettings
    ocr: OCRSettings
    cache: CacheSettings


class _UniqueKeyLoader(yaml.SafeLoader):
    pass


def _mapping(loader: _UniqueKeyLoader, node: yaml.MappingNode, deep: bool = False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in result:
            raise ValueError(f"duplicate YAML key: {key}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


_UniqueKeyLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _mapping)


def load_config(path: str | Path) -> Settings:
    try:
        with Path(path).open(encoding="utf-8") as source:
            data = yaml.load(source, Loader=_UniqueKeyLoader)
    except (OSError, yaml.YAMLError, ValueError) as error:
        raise ValueError(f"invalid course-graph config: {error}") from error
    if not isinstance(data, dict):
        raise ValueError("course-graph config must contain one YAML mapping")
    try:
        return Settings.model_validate(data, strict=True)
    except Exception as error:
        raise ValueError(f"invalid course-graph config: {error}") from error


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    path = os.environ.get("COURSE_GRAPH_CONFIG", "config/dev.yaml")
    return load_config(path)


def configure_library_environment(settings: Settings) -> None:
    """Set third-party defaults before torch, BLAS, transformers, or Docling imports."""
    os.environ.setdefault("OMP_NUM_THREADS", str(settings.ocr.threads))
    os.environ.setdefault("MKL_NUM_THREADS", str(settings.ocr.threads))
    os.environ.setdefault("HF_HUB_DISABLE_XET", "1")
    os.environ.setdefault("HF_HUB_DOWNLOAD_TIMEOUT", "30")
