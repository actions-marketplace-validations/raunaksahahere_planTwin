from django.db import models


class Customer(models.Model):
    name = models.TextField()
    region = models.CharField(max_length=8)


class Order(models.Model):
    # db_index=False: Django would otherwise create its own index on customer_id, and the planted regression would not
    # remove the only index on that column. The custom index below is the one under test.
    customer = models.ForeignKey(Customer, on_delete=models.CASCADE, db_index=False)
    status = models.CharField(max_length=16)
    created_at = models.DateTimeField()
    total = models.DecimalField(max_digits=10, decimal_places=2)

    class Meta:
        indexes = [
            models.Index(fields=["customer"], name="orders_customer_idx"),
            models.Index(fields=["status"], name="orders_status_idx"),
        ]
