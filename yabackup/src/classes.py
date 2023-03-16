from dataclasses import dataclass


@dataclass
class BackupFile:
    """Backup file class."""

    slug: str
    name: str
    date: str
    size: str
    in_local: bool = False
    in_remote: bool = False
