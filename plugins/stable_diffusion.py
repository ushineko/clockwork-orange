#!/usr/bin/env python3
"""
Stable Diffusion Plugin for Clockwork Orange.
Generates wallpapers using local Stable Diffusion (requires diffusers/torch).
"""

import sys
import time
import os
import random
from datetime import datetime, timedelta
from pathlib import Path

# VENV SHIM: Check for dependencies and switch interpreter if needed
try:
    import torch
    from diffusers import StableDiffusionPipeline, DPMSolverMultistepScheduler
except ImportError:
    # Dependencies missing. Check if we have a dedicated venv.
    VENV_PYTHON = Path.home() / ".local/share/clockwork-orange/venv-sd/bin/python"
    
    # Avoid infinite loop if venv python also fails/is broken
    if VENV_PYTHON.exists() and sys.executable != str(VENV_PYTHON):
        # Re-execute this script using the venv python
        # We replace the current process image
        os.execv(str(VENV_PYTHON), [str(VENV_PYTHON)] + sys.argv)
    
    # If we are here, we are either in the venv (and imports failed) or no venv exists.
    # We will let the class definition proceed, but run() will handle the error reporting.
    pass

# Add parent directory to path
sys.path.append(str(Path(__file__).parent.parent))

from plugins.base import PluginBase
from plugins.blacklist import BlacklistManager
from plugins.history import HistoryManager


class StableDiffusionPlugin(PluginBase):
    def get_description(self) -> str:
        return "Generate wallpapers using local Stable Diffusion (Requires setup)"

    def get_config_schema(self) -> dict:
        return {
            "prompt": {
                "type": "string_list",
                "default": [
                    {"term": "breathtaking landscape, mountains, lake, sunset, 4k, photorealistic, serene", "enabled": True}
                ],
                "description": "Text Prompts (Randomly Selected)",
                "suggestions": [
                    "breathtaking landscape, mountains, lake, sunset, 4k, photorealistic, serene",
                    "abstract 3d geometric shapes, vibrant colors, 4k, raytracing",
                    "cyberpunk city street, rain, neon lights, 4k, detailed",
                    "deep space nebula, stars, galaxy, 8k, hubble style"
                ]
            },
            "negative_prompt": {
                "type": "string",
                "default": "blurry, low quality, distortion, ugly, text, watermark",
                "description": "Negative Prompt",
            },
            "model_id": {
                "type": "string",
                "default": "runwayml/stable-diffusion-v1-5",
                "description": "HuggingFace Model ID",
                "suggestions": [
                    "runwayml/stable-diffusion-v1-5",
                    "prompthero/openjourney",
                    "stabilityai/stable-diffusion-2-1",
                ],
            },
            "steps": {
                "type": "integer",
                "default": 30,
                "description": "Inference Steps",
            },
            "width": {
                "type": "integer",
                "default": 768,
                "description": "Generation Width (Base)",
                "suggestions": [512, 768, 960],
            },
            "height": {
                "type": "integer",
                "default": 512,
                "description": "Generation Height (Base)",
                "suggestions": [512, 768],
            },
            "upscale": {
                "type": "boolean",
                "default": True,
                "description": "Upscale to Wallpaper (1080p+)",
            },
            "safety_checker": {
                "type": "boolean",
                "default": True,
                "description": "Enable NSFW Safety Filter",
            },
            "guidance_scale": {
                "type": "integer",
                "default": 7,  # Default was 7.5 float, using int 7 for simplicity in gui
                "description": "Guidance Scale",
            },
            "download_dir": {
                "type": "string",
                "default": str(
                    Path.home() / "Pictures" / "Wallpapers" / "StableDiffusion"
                ),
                "description": "Output Directory",
                "widget": "directory_path",
            },
            "interval": {
                "type": "string",
                "default": "Daily",
                "description": "Generation Interval",
                "enum": ["Hourly", "Daily", "Weekly"],
            },
            "limit": {
                "type": "integer",
                "default": 1,
                "description": "Images per Run",
            },
            "max_files": {
                "type": "integer",
                "default": 50,
                "description": "Retention Limit",
            },
        }

    def run(self, config: dict) -> dict:
        # Check Dependencies at Runtime
        try:
            import torch
            from diffusers import StableDiffusionPipeline, DPMSolverMultistepScheduler
        except ImportError:
            return {
                "status": "error",
                "message": (
                    "Missing dependencies. Please run 'scripts/setup_stable_diffusion.sh' "
                    "to create the necessary virtual environment."
                ),
            }

        # Setup Config
        download_dir = Path(
            config.get(
                "download_dir",
                Path.home() / "Pictures" / "Wallpapers" / "StableDiffusion",
            )
        )
        download_dir.mkdir(parents=True, exist_ok=True)

        interval = config.get("interval", "Daily").lower()
        limit = int(config.get("limit", 1))
        force = config.get("force", False)

        # Managers
        self.blacklist_manager = BlacklistManager()
        self.history_manager = HistoryManager()

        # Handle Blacklist Action
        if config.get("action") == "process_blacklist":
            targets = config.get("targets", [])
            print(
                f"[StableDiffusion] Processing blacklist for {len(targets)} files...",
                file=sys.stderr,
            )
            self.blacklist_manager.process_files(targets, plugin_name="stable_diffusion")
            return {"status": "success", "message": "Blacklist processed"}
            
        # Handle Delete Action (No Blacklist)
        if config.get("action") == "delete_files":
            targets = config.get("targets", [])
            print(f"[StableDiffusion] Deleting {len(targets)} files...", file=sys.stderr)
            import os
            for t in targets:
                try:
                    os.remove(t)
                    print(f"[StableDiffusion] Deleted {Path(t).name}", file=sys.stderr)
                except Exception as e:
                    print(f"[StableDiffusion] Failed to delete {t}: {e}", file=sys.stderr)
            return {"status": "success", "message": "Files deleted"}
            
        # Handle Reset
        if config.get("reset", False):
            self._perform_reset(download_dir)

        # Check Interval
        if not force and not self._should_run(download_dir, interval):
            return {"status": "success", "path": str(download_dir)}

        # Parameters
        # Parse Prompts
        raw_prompt = config.get("prompt", "landscape")
        prompts_list = []
        
        if isinstance(raw_prompt, str):
            prompts_list = [raw_prompt]
        elif isinstance(raw_prompt, list):
            for item in raw_prompt:
                if isinstance(item, dict):
                    if item.get("enabled", True):
                        prompts_list.append(item.get("term", ""))
                elif isinstance(item, str):
                    prompts_list.append(item)
        
        # Fallback
        if not prompts_list:
            prompts_list = ["landscape"]

        print(f"[StableDiffusion] Loaded {len(prompts_list)} active prompts.", file=sys.stderr)

        negative_prompt = config.get("negative_prompt", "")
        model_id = config.get("model_id", "stabilityai/stable-diffusion-2-1-base")
        steps = int(config.get("steps", 30))
        guidance_scale = float(config.get("guidance_scale", 7.5))

        print(f"[StableDiffusion] Loading model: {model_id}...", file=sys.stderr)
        print(f"::PROGRESS:: 10 :: Loading Model (this may take a while)...", file=sys.stderr)

        # Load Pipeline
        try:
            # Check device
            device = "cuda" if torch.cuda.is_available() else "cpu"
            # Mac MPS support
            if torch.backends.mps.is_available():
                device = "mps"

            print(f"[StableDiffusion] Using device: {device}", file=sys.stderr)

            # Optimization for low RAM if on CUDA (int8/float16)
            torch_dtype = torch.float32
            if device == "cuda":
                torch_dtype = torch.float16

            pipe = StableDiffusionPipeline.from_pretrained(
                model_id, torch_dtype=torch_dtype
            )
            
            # Use DPM Scheduler for faster results
            pipe.scheduler = DPMSolverMultistepScheduler.from_config(pipe.scheduler.config)
            
            if not config.get("safety_checker", True):
                print("[StableDiffusion] Disabling Safety Checker...", file=sys.stderr)
                pipe.safety_checker = None
                pipe.requires_safety_checker = False
            
            pipe = pipe.to(device)
            
            # Enable attention slicing for lower memory usage
            pipe.enable_attention_slicing()

        except Exception as e:
            return {
                "status": "error",
                "message": f"Failed to load model: {str(e)}",
            }

        print(f"::PROGRESS:: 30 :: generating {limit} image(s)...", file=sys.stderr)

        generated_count = 0
        for i in range(limit):
            try:
                # Select Random Prompt
                prompt = random.choice(prompts_list)
                print(f"[StableDiffusion] Using Prompt: {prompt}", file=sys.stderr)

                progress = 30 + int((i / limit) * 60)
                print(
                    f"::PROGRESS:: {progress} :: Generating image {i+1}/{limit}...",
                    file=sys.stderr,
                )

                # Generate
                # Note: SD is trained on 512x512. Going too high can cause artifacting.
                # We use 768x512 (landscape) as a safe baseline for wallpapers.
                gen_width = int(config.get("width", 768))
                gen_height = int(config.get("height", 512))
                
                # Ensure multiple of 8 (requirement for VAE)
                gen_width = (gen_width // 8) * 8
                gen_height = (gen_height // 8) * 8
                
                output = pipe(
                    prompt,
                    negative_prompt=negative_prompt,
                    num_inference_steps=steps,
                    guidance_scale=guidance_scale,
                    width=gen_width,
                    height=gen_height,
                )
                
                image = output.images[0]
                
                # Check for NSFW
                # If safety_checker is active, it returns nsfw_content_detected list
                if hasattr(output, "nsfw_content_detected") and output.nsfw_content_detected:
                    if output.nsfw_content_detected[0]:
                        print(f"[StableDiffusion] Safety checker blocked image. Skipping save.", file=sys.stderr)
                        continue

                # Upscale if requested (Simple Lanczos)
                if config.get("upscale", True):
                    target_w, target_h = 2560, 1440 # QHD Target
                    print(f"[StableDiffusion] Upscaling to {target_w}x{target_h}...", file=sys.stderr)
                    # Simple resize using LANCZOS (high quality down/upscale)
                    # For a true AI upscale we'd need another model, but this is good for basic use.
                    image = image.resize((target_w, target_h), 1) # PIL.Image.LANCZOS = 1

                # Create Filename
                timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
                filename = f"sd_{timestamp}_{i}.png"
                filepath = download_dir / filename

                # Save
                image.save(filepath)

                # Record in History
                # We do NOT record history for generated images as they are random/unique
                # and don't match a hashable source effectively.
                # fake_url = f"generated://stable_diffusion/{filename}"
                # self.history_manager.add_entry(
                #     fake_url, filepath, source="stable_diffusion"
                # )

                print(f"::IMAGE_SAVED:: {filepath}", file=sys.stderr)
                print(f"[StableDiffusion] Saved {filename}", file=sys.stderr)
                generated_count += 1

            except Exception as e:
                print(f"[StableDiffusion] Generation failed: {e}", file=sys.stderr)

        # Update run time
        self._update_last_run(download_dir)

        # Cleanup
        max_files = int(config.get("max_files", 50))
        print(f"::PROGRESS:: 95 :: Cleaning up old files...", file=sys.stderr)
        self._cleanup_old_files(download_dir, max_files)

        print(f"::PROGRESS:: 100 :: Done!", file=sys.stderr)

        if generated_count > 0:
            return {"status": "success", "path": str(download_dir)}
        
        return {"status": "error", "message": "Failed to generate any images"}

    def _cleanup_old_files(self, download_dir: Path, max_files: int):
        try:
            # Glob for .png files (SD output)
            files = sorted(
                [f for f in download_dir.glob("*.png")], key=lambda f: f.stat().st_mtime
            )

            if len(files) > max_files:
                to_remove = len(files) - max_files
                print(
                    f"[StableDiffusion] Cleaning up {to_remove} old images...",
                    file=sys.stderr,
                )
                for f in files[:to_remove]:
                    f.unlink()
                    print(f"[StableDiffusion] Removed {f.name}", file=sys.stderr)

        except Exception as e:
            print(f"[StableDiffusion] Cleanup failed: {e}", file=sys.stderr)

    def _should_run(self, download_dir: Path, interval: str) -> bool:
        if interval == "always":
            return True

        timestamp_file = download_dir / ".last_run"
        if not timestamp_file.exists():
            return True

        try:
            last_run = float(timestamp_file.read_text().strip())
            last_run_dt = datetime.fromtimestamp(last_run)
            now = datetime.now()

            delta = now - last_run_dt
            
            target_delta = None
            if interval == "hourly":
                target_delta = timedelta(hours=1)
            elif interval == "daily":
                target_delta = timedelta(days=1)
            elif interval == "weekly":
                target_delta = timedelta(weeks=1)
            
            if target_delta:
                if delta > target_delta:
                    print(f"[StableDiffusion] Interval reached (Last run: {delta} ago)", file=sys.stderr)
                    return True
                else:
                    remaining = target_delta - delta
                    # Format remaining time friendly
                    rem_str = str(remaining).split('.')[0]
                    print(f"[StableDiffusion] Skipping run. Time remaining: {rem_str} (Interval: {interval})", file=sys.stderr)
                    return False

        except Exception as e:
            print(f"[StableDiffusion] Interval check failed ({e}). Defaulting to RUN.", file=sys.stderr)
            return True

        return False

    def _update_last_run(self, download_dir: Path):
        timestamp_file = download_dir / ".last_run"
        timestamp_file.write_text(str(time.time()))

    def _perform_reset(self, download_dir):
        print(f"[StableDiffusion] Resetting directory {download_dir}...", file=sys.stderr)
        try:
            import shutil
            for item in download_dir.iterdir():
                if item.is_file():
                    item.unlink()
                elif item.is_dir():
                    shutil.rmtree(item)
        except Exception as e:
            print(f"[StableDiffusion] Reset failed: {e}", file=sys.stderr)


if __name__ == "__main__":
    plugin = StableDiffusionPlugin()
    plugin.main()
