import time
import subprocess
import threading
from dataclasses import dataclass
from typing import List


@dataclass
class WorkerConfig:
    port: int
    redis_port: int
    discovery_port: int
    bin_dir: str = "./bin"
    redis_bin: str = "redis-server"
    enable_gossip: bool = True
    virtual_nodes: int = 10
    replication: int = 3
    write_quorum: int = 3
    read_quorum: int = 3
    gossip_peers: int = 2
    gossip_interval: int = 5  # seconds


@dataclass
class ClientConfig:
    server_addr: str = "localhost:60050"


@dataclass
class WorkerBehavior:
    start_delay: float
    lifetime: float


class Behavior:
    def __init__(self):
        self.inp = []

    def get(self, key: str):
        self.inp.append(f"g {key}\n")

    def put(self, key: str, val: str):
        self.inp.append(f"p {key} {val}\n")

    def wait(self, waittime: int):
        self.inp.append(f"w {waittime}\n")


def run_worker(conf: WorkerConfig, behavior: WorkerBehavior):
    time.sleep(behavior.start_delay)

    redis_proc = subprocess.Popen([conf.redis_bin, "--port", str(conf.redis_port)],
                                  stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL)

    cmd = [
        f"{conf.bin_dir}/worker",
        f"-port={conf.port}",
        f"-redisport={conf.redis_port}",
        f"-replication={conf.replication}",
        f"-writequorum={conf.write_quorum}",
        f"-readquorum={conf.read_quorum}",
        f"-virtualnodes={conf.virtual_nodes}",
        f"-gossip={conf.gossip_interval}s",
        f"-peers={conf.gossip_peers}",
        f"-enablegossip={str(conf.enable_gossip).lower()}",
        f"-discovery={conf.discovery_port}",
    ]
    # worker_proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    worker_proc = subprocess.Popen(cmd)

    try:
        time.sleep(behavior.lifetime)
    finally:
        worker_proc.kill()
        redis_proc.kill()


def run_client(conf: ClientConfig, behavior: Behavior):
    def handle():
        process = subprocess.Popen(
            [f"./bin/autoclient", "-server", conf.server_addr],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            universal_newlines=True
        )

        input_script = "".join(behavior.inp + ["q\n"])
        try:
            output, _ = process.communicate(input=input_script, timeout=10)
            print(f"Client output:\n{output}")
        except subprocess.TimeoutExpired:
            print("Client timed out.")
            process.kill()

    threading.Thread(target=handle).start()


# Example usage
if __name__ == "__main__":
    NUM_WORKERS = 3
    BASE_PORT = 60050
    BASE_REDIS = 63079
    DISCOVERY_PORT = 63179

    workers = []
    for i in range(NUM_WORKERS):
        wc = WorkerConfig(
            port=BASE_PORT + i,
            redis_port=BASE_REDIS + i,
            discovery_port=DISCOVERY_PORT
        )
        wb = WorkerBehavior(start_delay=0, lifetime=50)  # staggered start
        t = threading.Thread(target=run_worker, args=(wc, wb))
        t.start()
        workers.append(t)
    time.sleep(1)
    b1 = Behavior()
    b1.wait(5)
    b1.put("a", "1")
    b1.wait(1)
    b1.get("a")

    b2 = Behavior()
    b2.wait(5)
    b2.put("b", "2")
    b2.get("b")

    run_client(ClientConfig(), b1)

    for _ in range(10):
        run_client(ClientConfig(), b2)

    # Wait for all workers to finish
    for t in workers:
        t.join()

    print("All workers finished.")
