import ctypes
import os
import sys

def set_wallpaper_windows(image_path):
    print(f"Attempting to set wallpaper to: {image_path}")
    if not os.path.exists(image_path):
        print("Error: File does not exist.")
        return False
        
    abs_path = os.path.abspath(image_path)
    print(f"Absolute path: {abs_path}")
    
    # SPI_SETDESKWALLPAPER = 20
    # SPIF_UPDATEINIFILE = 0x01
    # SPIF_SENDCHANGE = 0x02
    SPI_SETDESKWALLPAPER = 20
    SPIF_UPDATEINIFILE = 1
    SPIF_SENDWININICHANGE = 2
    
    try:
        # Pvoid is effectively a string pointer for the path in this call
        # the parameters are (Action, uiParam, pvParam, fWinIni)
        res = ctypes.windll.user32.SystemParametersInfoW(
            SPI_SETDESKWALLPAPER, 
            0, 
            abs_path, 
            SPIF_UPDATEINIFILE | SPIF_SENDWININICHANGE
        )
        if res:
            print("Successfully called SystemParametersInfoW")
            return True
        else:
            print(f"SystemParametersInfoW failed. Error code: {ctypes.GetLastError()}")
            return False
    except Exception as e:
        print(f"Exception: {e}")
        return False

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python research_wallpaper.py <image_path>")
        # Create a dummy image if none provided?
        sys.exit(1)
    
    image = sys.argv[1]
    set_wallpaper_windows(image)
