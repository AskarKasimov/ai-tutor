from setuptools import find_packages, setup

setup(
    name="ai-tutor-scoring",
    version="0.1.0",
    description="Standalone rubric-aware LLM scoring module for AI Tutor",
    package_dir={"": "src"},
    packages=find_packages(where="src"),
    python_requires=">=3.12",
    install_requires=["pydantic>=2.7,<3"],
)
