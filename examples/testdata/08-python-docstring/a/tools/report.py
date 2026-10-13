"""Weekly check report."""

import csv


def write_report(rows, path):
    """Write one row per check.

    Example row: {"subject": "Test Subject", "status": "resolved"}
    """
    with open(path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["subject", "status"])
        writer.writeheader()
        writer.writerows(rows)
