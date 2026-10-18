"""Progress of the graph build that is running now.

The service runs one build at a time (graph_slots), so one tracker is enough.
A pass reports its stage with the number of model batches it will run and
advances when each batch finishes, retries included; the admin sees
"stage i of n, step k of m". Stages without work keep total 0.
"""
import asyncio

STAGES = ("topics", "synonyms", "prerequisites", "attach", "questions")


class BuildProgress:
    def __init__(self, build_id: str):
        self.build_id = build_id
        self.stage, self.done, self.total = STAGES[0], 0, 0

    def start(self, stage: str, total: int):
        self.stage, self.done, self.total = stage, 0, total

    async def after(self, awaitable):
        """Await one batch and count it done."""
        try:
            return await awaitable
        finally:
            self.done = min(self.done + 1, self.total)

    def as_dict(self):
        return {"build_id": self.build_id, "stage": self.stage, "stage_index": STAGES.index(self.stage) + 1,
                "stage_count": len(STAGES), "done": self.done, "total": self.total}


class NoProgress(BuildProgress):
    """Progress of a pass called outside a tracked build."""
    def __init__(self):
        super().__init__("")


async def tracked(progress, stage, awaitables):
    """gather that reports the stage and advances per finished batch."""
    progress = progress or NoProgress()
    progress.start(stage, len(awaitables))
    return await asyncio.gather(*(progress.after(a) for a in awaitables))


current: BuildProgress | None = None
