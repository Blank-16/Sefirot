#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::{
    io::{BufRead, BufReader, Write},
    net::{SocketAddr, TcpStream},
    path::PathBuf,
    sync::{mpsc, Arc, Mutex},
    thread,
    time::Duration,
};
use tauri::{Manager, RunEvent};
use tauri_plugin_shell::{process::{CommandChild, CommandEvent}, ShellExt};

struct BackendState {
    child: Option<CommandChild>,
    url: Option<String>,
    token: Option<String>,
    exit_rx: Mutex<mpsc::Receiver<()>>,
}
struct BackendChild(Arc<Mutex<BackendState>>);

fn request_backend_shutdown(url: &str, token: &str) {
    let Some(port) = url.strip_prefix("http://127.0.0.1:").and_then(|s| s.parse::<u16>().ok()) else {
        return;
    };
    let address = SocketAddr::from(([127, 0, 0, 1], port));
    if let Ok(mut stream) = TcpStream::connect_timeout(&address, Duration::from_millis(500)) {
        let request = format!(
            "POST /api/shutdown HTTP/1.1\r\nHost: {}\r\nX-Vault-Token: {}\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
            address, token
        );
        let _ = stream.write_all(request.as_bytes());
    }
}

fn backend_responds(address: SocketAddr) -> bool {
    let Ok(mut stream) = TcpStream::connect_timeout(&address, Duration::from_millis(400)) else {
        return false;
    };
    let _ = stream.set_read_timeout(Some(Duration::from_millis(500)));
    let request = format!(
        "GET /api/health HTTP/1.1\r\nHost: {}\r\nConnection: close\r\n\r\n",
        address
    );
    if stream.write_all(request.as_bytes()).is_err() {
        return false;
    }
    let mut status = String::new();
    BufReader::new(stream).read_line(&mut status).is_ok() && status.contains(" 200 ")
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let data_dir = if let Some(path) = std::env::var_os("SEFIROT_DATA_DIR") {
                PathBuf::from(path)
            } else {
                app.path().app_local_data_dir()?
            };
            std::fs::create_dir_all(&data_dir)?;
            let data_dir = data_dir.to_string_lossy().to_string();
            let ready_file = std::env::temp_dir().join(format!("sefirot-ready-{}.txt", std::process::id()));
            let _ = std::fs::remove_file(&ready_file);
            let ready_file_arg = ready_file.to_string_lossy().to_string();

            let (exit_tx, exit_rx) = mpsc::channel();
            let (mut events, child) = app
                .shell()
                .sidecar("sefirot-server")?
                .args([
                    "--port", "0",
                    "--data-dir", data_dir.as_str(),
                    "--ready-file", ready_file_arg.as_str(),
                ])
                .spawn()?;
            app.manage(BackendChild(Arc::new(Mutex::new(BackendState {
                child: Some(child),
                url: None,
                token: None,
                exit_rx: Mutex::new(exit_rx),
            }))));

            let window = app
                .get_webview_window("main")
                .ok_or_else(|| std::io::Error::new(std::io::ErrorKind::NotFound, "main webview window was not created"))?;

            tauri::async_runtime::spawn(async move {
                while let Some(event) = events.recv().await {
                    match event {
                        CommandEvent::Stdout(bytes) | CommandEvent::Stderr(bytes) => {
                            let output = String::from_utf8_lossy(&bytes);
                            if !output.trim().is_empty() {
                                eprintln!("SEFIROT backend: {}", output.trim());
                            }
                        }
                        CommandEvent::Terminated(payload) => {
                            eprintln!("SEFIROT backend exited: {:?}", payload);
                            let _ = exit_tx.send(());
                            break;
                        }
                        _ => {}
                    }
                }
            });

            let backend_state = app.state::<BackendChild>().inner().0.clone();
            tauri::async_runtime::spawn(async move {
                for _ in 0..100 {
                    if let Ok(contents) = std::fs::read_to_string(&ready_file) {
                        let mut lines = contents.lines();
                        let url = lines.next().unwrap_or("").trim().to_string();
                        let token = lines.next().unwrap_or("").trim().to_string();
                        if let Some(port) = url.strip_prefix("http://127.0.0.1:").and_then(|s| s.parse::<u16>().ok()) {
                            let address = SocketAddr::from(([127, 0, 0, 1], port));
                            if !token.is_empty() && backend_responds(address) {
                                if let Ok(mut state) = backend_state.lock() {
                                    state.url = Some(url.clone());
                                    state.token = Some(token);
                                }
                                if let Ok(url_value) = url.parse() {
                                    let _ = window.navigate(url_value);
                                }
                                let _ = std::fs::remove_file(&ready_file);
                                return;
                            }
                        }
                    }
                    thread::sleep(Duration::from_millis(100));
                }
                let _ = window.eval("document.body.innerHTML = '<main style=\"font:14px system-ui;padding:3rem\"><h2>SEFIROT backend did not start</h2><p>Check the application logs and verify that the bundled server is present.</p></main>';");
                let _ = std::fs::remove_file(&ready_file);
            });
            Ok(())
        })
        .on_event(|app, event| {
            if matches!(event, RunEvent::Exit) {
                if let Some(state) = app.try_state::<BackendChild>() {
                    if let Ok(mut backend) = state.0.lock() {
                        if let (Some(url), Some(token)) = (backend.url.as_deref(), backend.token.as_deref()) {
                            request_backend_shutdown(url, token);
                            if let Ok(receiver) = backend.exit_rx.lock() {
                                let _ = receiver.recv_timeout(Duration::from_secs(5));
                            }
                        }
                        if let Some(mut child) = backend.child.take() {
                            let _ = child.kill();
                        }
                    }
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("failed to run SEFIROT desktop shell");
}
