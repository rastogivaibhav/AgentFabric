# agentfabric-auto-instrument-start
def _boot():
    try:
        from agentfabric.auto_instrument import AutoInstrumentor

        AutoInstrumentor().run()
    except Exception:
        pass


_boot()
# agentfabric-auto-instrument-end
