import numpy as np
from fastapi.testclient import TestClient

import api


class FakeModel:
    tts_model = type("TTS", (), {"sample_rate": 48000})()

    def generate(self, **kwargs):
        self.kwargs = kwargs
        return np.zeros(480, dtype=np.float32)


def test_synthesize_returns_wav(monkeypatch):
    model = FakeModel()
    monkeypatch.setattr(api, "_load_model", lambda: model)
    response = TestClient(api.app).post("/synthesize", json={"text": "Привет, мир!"})
    assert response.status_code == 200
    assert response.headers["content-type"] == "audio/wav"
    assert response.content[:4] == b"RIFF"
    assert model.kwargs["text"] == "Привет, мир!"


def test_synthesize_rejects_empty_text():
    response = TestClient(api.app).post("/synthesize", json={"text": ""})
    assert response.status_code == 422
    response = TestClient(api.app).post("/synthesize", json={"text": "   "})
    assert response.status_code == 422
