import subprocess
from pathlib import Path
from os import environ
import resource

if __name__ == "__main__":
    resource.setrlimit(resource.RLIMIT_STACK, (-1, -1))

    index = int(environ["CLOUD_RUN_TASK_INDEX"])
    count = int(environ["CLOUD_RUN_TASK_COUNT"])

    tomls = sorted(list(filter(lambda p: not p.match('test/**/info.toml'), Path('./library-checker-problems/').glob('**/info.toml'))))[index::count]

    print("tomls: ", tomls)

    subprocess.run(
        ["./library-checker-problems/generate.py", "--only-html"] +
        list(map(lambda p: str(p.absolute()), tomls)),
        check=True
    )

    API_HOST = "apiv1.yosupo.jp:443"
    API_USER = "judge"
    API_PASS = environ["API_PASS"]
    MINIO_HOST = environ["MINIO_HOST"]
    MINIO_ID = environ["MINIO_ID"]
    MINIO_SECRET = environ["MINIO_SECRET"]
    MINIO_BUCKET = environ["MINIO_BUCKET"]

    subprocess.run(
        ["./uploader"] +
        ["-apihost", API_HOST] +
        ["-apiuser", API_USER] +
        ["-apipass", API_PASS] +
        ["-miniohost", MINIO_HOST] +
        ["-minioid", MINIO_ID] +
        ["-miniokey", MINIO_SECRET] +
        ["-miniobucket", MINIO_BUCKET] +
        ["-dir", "./library-checker-problems"] +
        ["-tls"] +
        tomls,
        check=True
    )