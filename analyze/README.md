# 토큰 사용량 분석

## 실행

```bash
cd analyze
pip install -r requirements.txt
streamlit run app.py --server.port 9090
```

브라우저에서 http://localhost:9090 접속.

## 기능

- CSV 선택 후 **평균 토큰 금액** ($/token), **1달러당 평균 토큰량** (tokens/$) 계산
- CSV는 `Total Tokens`, `Cost` 열 필요 (Cost 단위: $)
