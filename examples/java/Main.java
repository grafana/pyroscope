class Main {
  public static int work(int n) {
    int i = 0;
    for (i = 0; i < n; i++) {}
    return i;
  }

  public static void fastFunction() {
    work(20000);
  }

  public static void slowFunction() {
    work(80000);
  }

  public static void main(String[] args) {
    int i = 0;
    while (true) {
      fastFunction();
      slowFunction();
      i++;
    }
  }
}
