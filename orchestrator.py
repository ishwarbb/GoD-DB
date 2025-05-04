import time
import subprocess
import threading
from dataclasses import dataclass
from typing import List
import re
import random
import string
import matplotlib.pyplot as plt
import time


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
    intervals = []


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
    # worker_proc = subprocess.Popen(cmd)

    # try:
    #     time.sleep(behavior.lifetime)
    # finally:
    #     worker_proc.kill()
    #     redis_proc.kill()

    # do it for intervals
    for interval in behavior.intervals:
        time.sleep(interval[0])
        worker_proc = subprocess.Popen(cmd)
        time.sleep(interval[1])
        worker_proc.kill()
    redis_proc.kill()



def run_client(conf: ClientConfig, behavior: Behavior, results, lock):
    def handle():
        # import resource
        # soft_limit, hard_limit = resource.getrlimit(resource.RLIMIT_NOFILE)
        # print(soft_limit, hard_limit)
        # import os
        # pid = os.getpid()
        # fd_dir = f"/proc/{pid}/fd"
        # num_fds = len(os.listdir(fd_dir))
        # print(f"Number of open FDs: {num_fds}")
        process = subprocess.Popen(
            [f"./bin/autoclient", "-server", conf.server_addr],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            universal_newlines=True,
            close_fds=True
        )

        input_script = "".join(behavior.inp + ["q\n"])
        try:
            output, _ = process.communicate(input=input_script, timeout=150)
            get_matches = re.findall("^get_latency: .*", output, re.MULTILINE)
            put_matches = re.findall("^put_latency: .*", output, re.MULTILINE)
            # print(get_matches)
            put_lat = [float(x.split(' ')[1]) for x in put_matches]
            get_lat = [float(x.split(' ')[1]) for x in get_matches]

            print(f"Client output:\n{output}")
        except subprocess.TimeoutExpired:
            print("Client timed out.")
            process.kill()
        return put_lat, get_lat
    put_lat, get_lat = handle()
    with lock:
        results.append((put_lat, get_lat))
        # print(results)
    # threading.Thread(target=handle).start()

def test1():
    avg_puts = []
    avg_gets = []
    for num_rep in range(3, 15):
        NUM_WORKERS = 50
        BASE_PORT = 60050
        BASE_REDIS = 63079
        DISCOVERY_PORT = 63179
        print("Num replication: ", num_rep)
        print("Num workers: ", NUM_WORKERS)

        workers = []
        for i in range(NUM_WORKERS):
            wc = WorkerConfig(
                port=BASE_PORT + i,
                redis_port=BASE_REDIS + i,
                discovery_port=DISCOVERY_PORT,
                replication=num_rep,
                write_quorum=num_rep,
            )
            wb = WorkerBehavior(start_delay=0, lifetime=10)  # staggered start
            wb.intervals = [[0, 8]]
            t = threading.Thread(target=run_worker, args=(wc, wb))
            t.start()
            workers.append(t)
        # time.sleep(1)
        # b1 = Behavior()
        # b1.wait(5)
        # b1.put("a", "1")
        # b1.wait(1)
        # b1.get("a")

        b2 = Behavior()
        # b2.wait(1)
        b2.put("b", "2")
        b2.get("b")

        # run_client(ClientConfig(), b1)

        # for _ in range(1000):
        #     # time.sleep(1)
        #     run_client(ClientConfig(), b2)
        
        # run clients concurrently
        start_time = time.time()
        clients = []
        results = []
        lock = threading.Lock()
        for i in range(5):
            cc = ClientConfig(server_addr=f'localhost:{BASE_PORT + random.randint(0, NUM_WORKERS - 1)}')
            b2 = Behavior()
            # b2.wait(1)
            ke = ''.join(random.choices(string.ascii_uppercase + string.digits, k=10))  # random string
            b2.put(ke, "2")
            b2.get(ke)
            # run_client returns get_lat and put_lat, average them across threads
            
            t = threading.Thread(target=run_client, args=(cc, b2, results, lock))
            t.start()
            clients.append(t)
        for t in clients:
            t.join()
        # print(results)
        avg_get = 0
        avg_put = 0
        for res in results:
            avg_get += res[0][0]
            avg_put += res[1][0]
        avg_get /= len(results)
        avg_put /= len(results)
        print(f"Average get latency: {avg_get}")
        print(f"Average put latency: {avg_put}")
        print(f"total time: {time.time() - start_time}")
        avg_puts.append(avg_put)
        avg_gets.append(avg_get)


        # Wait for all workers to finish
        for t in workers:
            t.join()

        print("All workers finished.")
    print("None???")
    plt.plot(
        range(3, 15), avg_gets, label="Average Get Latency")
    plt.plot(range(3, 15), avg_puts, label="Average Put Latency")
    plt.xlabel("Replication Factor")
    plt.ylabel("Latency (ms)")
    plt.title("Latency vs. Replication Factor")
    plt.legend()
    plt.savefig("latency.png")
    plt.show()
    print("Plot done")

def test2():
    avg_puts = []
    avg_gets = []
    NUM_WORKERS = 7
    BASE_PORT = 60050
    BASE_REDIS = 63079
    DISCOVERY_PORT = 63179
    # print("Num replication: ", num_rep)
    print("Num workers: ", NUM_WORKERS)

    workers = []
    for i in range(NUM_WORKERS):
        wc = WorkerConfig(
            port=BASE_PORT + i,
            redis_port=BASE_REDIS + i,
            discovery_port=DISCOVERY_PORT,
            replication=3,
        )
        wb = WorkerBehavior(start_delay=0, lifetime=10)  # staggered start
        # wb.intervals = [[0, 10 + (i + 1) * 5]]
        wb.intervals = [[0, 50]]
        t = threading.Thread(target=run_worker, args=(wc, wb))
        t.start()
        workers.append(t)
    time.sleep(10)

    start_time = time.time()
    clients = []
    results = []
    lock = threading.Lock()
    for i in range(250):
        cc = ClientConfig(server_addr=f'localhost:{BASE_PORT + random.randint(0, NUM_WORKERS - 1)}')
        b2 = Behavior()
        # b2.wait(1)
        for _ in range(2 * NUM_WORKERS):
            b2.wait(1)
            ke = ''.join(random.choices(string.ascii_uppercase + string.digits, k=10))  # random string
            b2.put(ke, "2")
            b2.get(ke)
            # run_client returns get_lat and put_lat, average them across threads
        
        t = threading.Thread(target=run_client, args=(cc, b2, results, lock))
        t.start()
        clients.append(t)
    for t in clients:
        t.join()
    # print(results)
    avg_puts = [0 for _ in range(2 * NUM_WORKERS)]
    avg_gets = [0 for _ in range(2 * NUM_WORKERS)]
    for x in results:
        for i in range(2 * NUM_WORKERS):
            avg_puts[i] += x[0][i]
            avg_gets[i] += x[1][i]
    for i in range(2 * NUM_WORKERS):
        avg_puts[i] /= len(results)
        avg_gets[i] /= len(results)
    print(f"Average get latency: {avg_gets}")
    print(f"Average put latency: {avg_puts}")
    print(f"total time: {time.time() - start_time}")



    # Wait for all workers to finish
    for t in workers:
        t.join()

    print("All workers finished.")
    # print("None???")
    # plt.plot(
    #     range(3, 15), avg_gets, label="Average Get Latency")
    # plt.plot(range(3, 15), avg_puts, label="Average Put Latency")
    # plt.xlabel("Replication Factor")
    # plt.ylabel("Latency (ms)")
    # plt.title("Latency vs. Replication Factor")
    # plt.legend()
    # plt.savefig("latency.png")
    # plt.show()
    # print("Plot done")
    
# Example usage
if __name__ == "__main__":
    test2()
    
