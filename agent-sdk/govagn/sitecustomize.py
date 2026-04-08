# govagn-auto-instrument-start
def _boot():
    try:
        from govagn.auto_instrument import AutoInstrumentor

        AutoInstrumentor().run()
    except Exception:
        pass


_boot()
# govagn-auto-instrument-end
