"""Minimal Django settings for the example. The database comes from DATABASE_URL, which PlanTwin sets to the twin."""
import os
import secrets
from urllib.parse import unquote, urlparse

# Read from the environment. A random key is generated when none is set, so no secret lives in the repository.
SECRET_KEY = os.environ.get("DJANGO_SECRET_KEY") or secrets.token_urlsafe(48)
DEBUG = False
INSTALLED_APPS = ["django.contrib.contenttypes", "shop"]
DEFAULT_AUTO_FIELD = "django.db.models.AutoField"
USE_TZ = True


def _database_from_url(url):
    parts = urlparse(url)
    return {
        "ENGINE": "django.db.backends.postgresql",
        "NAME": parts.path.lstrip("/"),
        "USER": unquote(parts.username or ""),
        "PASSWORD": unquote(parts.password or ""),
        "HOST": parts.hostname or "localhost",
        "PORT": str(parts.port or 5432),
    }


DATABASES = {
    "default": _database_from_url(os.environ.get("DATABASE_URL", "postgres://postgres@localhost:5432/postgres")),
}
