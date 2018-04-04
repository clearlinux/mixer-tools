mixer init --clear-version 21650 --mix-version 10
wait
mixer build chroots
wait
mixer build update
wait
mixer build image --format=1
wait
python3 dm-verity-impl-1.py release.img
