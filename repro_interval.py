import sys
import time
from datetime import datetime, timedelta
from pathlib import Path


# Simulation of the class method
def should_run(last_run_ts, interval):
    # normalize
    interval = interval.lower()

    if interval == "always":
        return True

    # Simulate parsing
    try:
        last_run = float(last_run_ts)
        last_run_dt = datetime.fromtimestamp(last_run)
        now = datetime.now()

        print(f"Now: {now}, Last Run: {last_run_dt}, Interval: {interval}")

        if interval == "hourly":
            return now - last_run_dt > timedelta(hours=1)
        elif interval == "daily":
            return now - last_run_dt > timedelta(days=1)
        elif interval == "weekly":
            return now - last_run_dt > timedelta(weeks=1)
    except Exception as e:
        print(f"Exception: {e}")
        return True

    print("Fall through return False")
    return False


# Test Cases
now_ts = time.time()
one_hour_ago = now_ts - 3601
two_hours_ago = now_ts - 7201
yesterday = now_ts - 86401

print(
    f"Test 1: Hourly, 1h ago -> {should_run(one_hour_ago, 'Hourly')}"
)  # Should be True
print(
    f"Test 2: Hourly, 0.5h ago -> {should_run(now_ts - 1800, 'Hourly')}"
)  # Should be False
print(f"Test 3: Daily, 25h ago -> {should_run(yesterday, 'Daily')}")  # Should be True
print(
    f"Test 4: Daily, 2h ago -> {should_run(two_hours_ago, 'Daily')}"
)  # Should be False
print(
    f"Test 5: Unknown -> {should_run(yesterday, 'Monthly')}"
)  # Should be False (fallthrough)
