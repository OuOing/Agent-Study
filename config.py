"""Runtime configuration loaded from environment variables."""

import os


MODEL_API_KEY = os.getenv("MODEL_API_KEY", "")
MODEL_NAME = os.getenv("MODEL_NAME", "demo-model")

