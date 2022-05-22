# Traffic Operator

## Koncepcja:

Operator korzystający z jednego CRD, zawierającego definicję deploymentu który operator ma kontrolować i dwa poziomy
ruchu sieciowego, których nie powinny przekraczać odpowiednio pody i nody.
Startując aplikację, operator konfiguruje pod affinity tak żeby dla nowych podów nie wybierało nodów oznaczonych 
taintem operatora. Równocześnie operator kontroluje poziom ruchu w danym nodzie i jeśli ograniczenie zostanie 
przekroczone oznacza ten node odpowiednim taintem.

### Problemy:
- pomiar ruchu sieciowego:
    - na poziomie nodów - coś typu prometheus node exporter / node problem detector?
- czy operator konfiguruje autoscailing bazowany na limicie ruchu dla podów, 
    czy sam powinien tworzyć nowe pody jeśli któryś node przekracza limity?
    - pomiar ruchu na poziomie podów - czy robią to same czy jakiś envoy czy coś?