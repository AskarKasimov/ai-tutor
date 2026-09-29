"""HTTP contract tests that do not download model weights."""

from dataclasses import dataclass

from fastapi.testclient import TestClient

from api import create_app


@dataclass
class FakeResult:
    text: str


class FakeModel:
    def transcribe(self, path):
        with open(path, "rb") as audio:
            content = audio.read()
        if content == b"long":
            raise ValueError("Too long wav file, use 'transcribe_longform' method.")
        if content == b"invalid":
            raise RuntimeError("Failed to load audio")
        return FakeResult("Привет, мир!")


def test_transcribe_returns_text():
    with TestClient(create_app(model_loader=FakeModel)) as client:
        response = client.post("/transcribe", files={"file": ("voice.ogg", b"audio")})
    assert response.status_code == 200
    assert response.json()["text"] == "Привет, мир!"


def test_empty_audio_is_rejected():
    with TestClient(create_app(model_loader=FakeModel)) as client:
        response = client.post("/transcribe", files={"file": ("voice.wav", b"")})
    assert response.status_code == 422


def test_oversized_audio_is_rejected():
    with TestClient(create_app(model_loader=FakeModel, max_upload_bytes=3)) as client:
        response = client.post("/transcribe", files={"file": ("voice.wav", b"audio")})
    assert response.status_code == 413


def test_invalid_and_long_audio_are_rejected():
    with TestClient(create_app(model_loader=FakeModel)) as client:
        invalid = client.post("/transcribe", files={"file": ("voice.wav", b"invalid")})
        long = client.post("/transcribe", files={"file": ("voice.wav", b"long")})
    assert invalid.status_code == 422
    assert long.status_code == 422
