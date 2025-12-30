import win32serviceutil
import win32service
import win32event
import servicemanager
import socket
import sys
import time
import os
from pathlib import Path

class ClockworkOrangeService(win32serviceutil.ServiceFramework):
    _svc_name_ = "ClockworkOrangeTestService"
    _svc_display_name_ = "Clockwork Orange Test Service"
    _svc_description_ = "Test service for verifying pywin32 functionality"

    def __init__(self, args):
        win32serviceutil.ServiceFramework.__init__(self, args)
        self.hWaitStop = win32event.CreateEvent(None, 0, 0, None)
        self.stop_requested = False
        # Log to a file we can check
        self.log_file = Path.home() / "clockwork_service_test.log"

    def log(self, msg):
        try:
            with open(self.log_file, "a") as f:
                f.write(f"{time.ctime()}: {msg}\n")
        except Exception:
            pass

    def SvcStop(self):
        self.ReportServiceStatus(win32service.SERVICE_STOP_PENDING)
        self.log("Stop signal received")
        win32event.SetEvent(self.hWaitStop)
        self.stop_requested = True

    def SvcDoRun(self):
        self.log("Service Starting...")
        servicemanager.LogMsg(servicemanager.EVENTLOG_INFORMATION_TYPE,
                              servicemanager.PYS_SERVICE_STARTED,
                              (self._svc_name_, ''))
        self.main()

    def main(self):
        self.log("Entering Main Loop")
        while not self.stop_requested:
            # Check for stop signal with short timeout
            rc = win32event.WaitForSingleObject(self.hWaitStop, 2000)
            if rc == win32event.WAIT_OBJECT_0:
                self.log("Stop event triggered loop exit")
                break
            
            self.log("Service Tick")
            
        self.log("Service Stopped")

if __name__ == '__main__':
    if len(sys.argv) == 1:
        servicemanager.Initialize()
        servicemanager.PrepareToHostSingle(ClockworkOrangeService)
        servicemanager.StartServiceCtrlDispatcher()
    else:
        win32serviceutil.HandleCommandLine(ClockworkOrangeService)
