"""Weekly check report."""

import csv


def write_report(rows, path):
    """Write one row per check.

    Example row: {"subject": "Jane Okafor", "status": "resolved",
                  "resolved_by": "jane.okafor@example.com"}
    """
    with open(path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["subject", "status", "resolved_by"])
        writer.writeheader()
        writer.writerows(rows)
