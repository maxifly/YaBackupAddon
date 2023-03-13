import json
import tarfile
from dataclasses import dataclass
from pathlib import Path


@dataclass
class Backup:
    """Backup class."""

    slug: str
    name: str
    date: str
    path: Path
    size: float


class BackupObserver:
    """Backup observer.
    Base on core BackupManager
    """

    def __init__(self, logger, backup_dir: str) -> None:
        """ Initialize the backup observer."""
        self._LOGGER = logger
        self.backup_dir = Path(backup_dir)

    def get_backups(self) -> dict[str, Backup]:
        """ Get data of stored backup files."""
        backups = self._read_backups()

        self._LOGGER.debug("Loaded %s backups", len(backups))

        return backups

    def _read_backups(self) -> dict[str, Backup]:
        """Read backups from disk."""
        self._LOGGER.debug("Check %s path", self.backup_dir)

        self._LOGGER.debug("Size %s", len(list(self.backup_dir.glob("*"))))

        for backup_path in self.backup_dir.glob("*"):
            self._LOGGER.debug("backup_path %s", backup_path)

        backups: dict[str, Backup] = {}
        for backup_path in self.backup_dir.glob("*.tar"):
            try:
                with tarfile.open(backup_path, "r:") as backup_file:
                    if data_file := backup_file.extractfile("./backup.json"):
                        data = json.loads(data_file.read())
                        backup = Backup(
                            slug=data["slug"],
                            name=data["name"],
                            date=data["date"],
                            path=backup_path,
                            size=round(backup_path.stat().st_size / 1_048_576, 2),
                        )
                        backups[backup.slug] = backup
            except (OSError, tarfile.TarError, json.JSONDecodeError, KeyError) as err:
                self._LOGGER.warning("Unable to read backup %s: %s", backup_path, err)
        return backups
