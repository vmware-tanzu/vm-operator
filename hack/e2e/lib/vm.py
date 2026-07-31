"""pyVmomi VM lifecycle helpers used by clone-pykmip-vm.py."""

import time

from pyVmomi import vim


def wait_for_task(task, timeout: int = 600, poll_interval: float = 2.0):
    """Block until task reaches success or error state.

    Returns task.info.result on success. Raises RuntimeError on vSphere error,
    TimeoutError if the task does not complete within *timeout* seconds.
    """
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        state = task.info.state
        if state == "success":
            return task.info.result
        if state == "error":
            err = task.info.error
            msg = getattr(err, "msg", str(err))
            raise RuntimeError(f"Task failed: {msg}")
        time.sleep(poll_interval)
    raise TimeoutError(
        f"Task did not complete within {timeout}s (last state: {task.info.state})"
    )


def CloneVM(src_vm, name: str, relocate_spec, isPowerOn: bool = False, configSpec=None):
    """Clone *src_vm* into a new VM named *name*.

    Places the clone according to *relocate_spec* and applies *configSpec* when
    provided. Returns the new VM's managed object reference.
    """
    clone_spec = vim.vm.CloneSpec(
        location=relocate_spec,
        powerOn=isPowerOn,
        template=False,
    )
    if configSpec is not None:
        clone_spec.config = configSpec
    task = src_vm.CloneVM_Task(folder=src_vm.parent, name=name, spec=clone_spec)
    return wait_for_task(task, timeout=600)


def ReconfigureVM(vm, config_spec, timeout: int = 300):
    """Apply *config_spec* to *vm*, blocking until the reconfigure task completes."""
    task = vm.ReconfigVM_Task(spec=config_spec)
    wait_for_task(task, timeout=timeout)


def PowerOnVM(vm, timeout: int = 300):
    """Power on *vm*, blocking until the power-on task completes."""
    task = vm.PowerOnVM_Task()
    wait_for_task(task, timeout=timeout)
